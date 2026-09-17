package releaseloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v4/stdlib"
	"github.com/lib/pq"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

const failed = "Failed"

type ReleaseLoader struct {
	// context in a struct violates golang best practices; but the releaseloader lifecycle is
	// within the scope of the context, it will never be reused, and keeping it in the object
	// keeps method calls more legible. so; best practices consciously overridden. [lmeyer]
	ctx           context.Context
	db            *db.DB
	bqClient      *bqcachedclient.Client
	httpClient    *http.Client
	releases      map[string]v1.Release
	architectures []string
	projects      []PayloadProject
	errors        []error
}

func New(ctx context.Context, dbc *db.DB, bqClient *bqcachedclient.Client, releases, architectures []string, releaseConfigs []v1.Release) *ReleaseLoader {
	configForRelease := make(map[string]v1.Release, len(releaseConfigs))
	for _, config := range releaseConfigs {
		if config.Capabilities[v1.PayloadTagsCap] {
			configForRelease[config.Release] = config
		}
	}
	if len(releases) > 0 {
		filteredRCs := make(map[string]v1.Release, len(releases))
		for _, release := range releases {
			if config, ok := configForRelease[release]; ok {
				filteredRCs[release] = config
			} else {
				log.Warningf("release %q is not configured to load payload tags", release)
			}
		}
		configForRelease = filteredRCs
	}

	return &ReleaseLoader{
		ctx:           ctx,
		db:            dbc,
		bqClient:      bqClient,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		releases:      configForRelease,
		architectures: architectures,
		projects:      []PayloadProject{&OCPProject{}, &OKDProject{}},
	}
}

func (r *ReleaseLoader) Name() string {
	return "releases"
}

func (r *ReleaseLoader) Errors() []error {
	return r.errors
}

type tagWithStream struct {
	Tag    ReleaseTag
	Stream ReleaseStream
}

const maxHTTPConcurrency = 10

func (r *ReleaseLoader) Load() {
	st := time.Now()

	allTags := r.fetchAllStreamTags()
	if len(allTags) == 0 {
		return
	}

	tagsToProcess, err := r.updatePhaseChangesAndFindNewTags(allTags)
	if err != nil {
		r.errors = append(r.errors, errors.Wrap(err, "error processing release tags"))
		return
	}

	builtTags := r.fetchTagDetails(tagsToProcess)

	r.applyBulkLabels(builtTags)

	if err := r.resolveAllPullRequests(builtTags); err != nil {
		r.errors = append(r.errors, errors.Wrap(err, "error resolving pull requests"))
		return
	}

	if err := r.bulkWriteReleaseTags(builtTags); err != nil {
		r.errors = append(r.errors, errors.Wrap(err, "error bulk writing release tags"))
	}

	log.WithFields(log.Fields{
		"fetched": len(allTags),
		"new":     len(builtTags),
		"elapsed": time.Since(st),
	}).Info("release loading complete")
}

func (r *ReleaseLoader) fetchAllStreamTags() []tagWithStream {
	work := make(chan ReleaseStream)
	results := make(chan tagWithStream, 100)

	go func() {
		defer close(work)
		for _, project := range r.projects {
			for _, rs := range buildReleaseStreams(r.releases, r.architectures, project) {
				work <- rs
			}
		}
	}()

	var wg sync.WaitGroup
	for range maxHTTPConcurrency {
		wg.Go(func() {
			for rs := range work {
				for _, tag := range r.fetchReleaseTags(rs) {
					results <- tagWithStream{Tag: tag, Stream: rs}
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var allTags []tagWithStream
	for ts := range results {
		allTags = append(allTags, ts)
	}
	return allTags
}

func (r *ReleaseLoader) fetchTagDetails(tagsToProcess []tagWithStream) []*models.ReleaseTag {
	work := make(chan tagWithStream)
	go func() {
		defer close(work)
		for _, ts := range tagsToProcess {
			work <- ts
		}
	}()

	results := make(chan *models.ReleaseTag, 100)
	var wg sync.WaitGroup
	for range maxHTTPConcurrency {
		wg.Go(func() {
			for ts := range work {
				log.WithField("tag", ts.Tag.Name).Info("fetching tag details from release controller")
				if rt := r.buildReleaseTag(ts.Stream, ts.Tag); rt != nil {
					results <- rt
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var built []*models.ReleaseTag
	for rt := range results {
		built = append(built, rt)
	}
	return built
}

func (r *ReleaseLoader) updatePhaseChangesAndFindNewTags(allTags []tagWithStream) ([]tagWithStream, error) {
	sqlDB, err := r.db.DB.DB()
	if err != nil {
		return nil, fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	cleanup, err := db.CopyToTempTable(r.ctx, conn, "tmp_release_tags", allTags,
		[]db.TempColumn[tagWithStream]{
			{Name: "tag_name", Type: "text NOT NULL", Value: func(ts *tagWithStream) any { return ts.Tag.Name }},
			{Name: "phase", Type: "text NOT NULL", Value: func(ts *tagWithStream) any { return ts.Tag.Phase }},
		},
	)
	defer cleanup()
	if err != nil {
		return nil, err
	}

	updateTag, err := conn.Exec(r.ctx, `
		UPDATE release_tags rt
		SET phase = tmp.phase, forced = true, updated_at = NOW()
		FROM tmp_release_tags tmp
		WHERE rt.release_tag = tmp.tag_name
		  AND rt.deleted_at IS NULL
		  AND rt.phase != ''
		  AND rt.phase != tmp.phase
	`)
	if err != nil {
		return nil, fmt.Errorf("bulk updating phase changes: %w", err)
	}

	rows, err := conn.Query(r.ctx, `
		SELECT DISTINCT tmp.tag_name
		FROM tmp_release_tags tmp
		LEFT JOIN release_tags rt ON rt.release_tag = tmp.tag_name AND rt.deleted_at IS NULL
		WHERE rt.id IS NULL
		  AND tmp.phase IN ('Accepted', 'Rejected')
	`)
	if err != nil {
		return nil, fmt.Errorf("querying new tags: %w", err)
	}
	newTagNames := sets.New[string]()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning new tag name: %w", err)
		}
		newTagNames.Insert(name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating new tags: %w", err)
	}

	var result []tagWithStream
	for _, ts := range allTags {
		if newTagNames.Has(ts.Tag.Name) {
			result = append(result, ts)
		}
	}

	log.WithFields(log.Fields{
		"total":         len(allTags),
		"new":           len(result),
		"phase_changed": updateTag.RowsAffected(),
	}).Info("processed release tags")

	return result, nil
}

func (r *ReleaseLoader) buildReleaseTag(rs ReleaseStream, tag ReleaseTag) *models.ReleaseTag {
	releaseDetails := r.fetchReleaseDetails(rs, tag)
	if releaseDetails == nil {
		return nil
	}
	return r.releaseDetailsToDB(rs, tag, *releaseDetails)
}

func (r *ReleaseLoader) resolveAllPullRequests(tags []*models.ReleaseTag) error {
	var allPRs []models.ReleasePullRequest
	for _, tag := range tags {
		allPRs = append(allPRs, tag.PullRequests...)
	}
	if len(allPRs) == 0 {
		return nil
	}

	sqlDB, err := r.db.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	cleanup, err := db.CopyToTempTable(r.ctx, conn, "tmp_release_prs", allPRs,
		[]db.TempColumn[models.ReleasePullRequest]{
			{Name: "url", Type: "text NOT NULL", Value: func(pr *models.ReleasePullRequest) any { return pr.URL }},
			{Name: "name", Type: "text NOT NULL", Value: func(pr *models.ReleasePullRequest) any { return pr.Name }},
			{Name: "description", Type: "text NOT NULL DEFAULT ''", Value: func(pr *models.ReleasePullRequest) any { return pr.Description }},
			{Name: "pull_request_id", Type: "text NOT NULL DEFAULT ''", Value: func(pr *models.ReleasePullRequest) any { return pr.PullRequestID }},
			{Name: "bug_url", Type: "text NOT NULL DEFAULT ''", Value: func(pr *models.ReleasePullRequest) any { return pr.BugURL }},
		},
	)
	defer cleanup()
	if err != nil {
		return fmt.Errorf("populating tmp_release_prs: %w", err)
	}

	upsertTag, err := conn.Exec(r.ctx, `
		INSERT INTO release_pull_requests (url, name, description, pull_request_id, bug_url, created_at, updated_at)
		SELECT DISTINCT ON (tmp.url, tmp.name) tmp.url, tmp.name, tmp.description, tmp.pull_request_id, tmp.bug_url, NOW(), NOW()
		FROM tmp_release_prs tmp
		ON CONFLICT (url, name) DO UPDATE SET
			description     = EXCLUDED.description,
			pull_request_id = EXCLUDED.pull_request_id,
			bug_url         = EXCLUDED.bug_url,
			updated_at      = NOW()
		WHERE release_pull_requests.description     IS DISTINCT FROM EXCLUDED.description
		   OR release_pull_requests.pull_request_id IS DISTINCT FROM EXCLUDED.pull_request_id
		   OR release_pull_requests.bug_url         IS DISTINCT FROM EXCLUDED.bug_url
	`)
	if err != nil {
		return fmt.Errorf("upserting release pull requests: %w", err)
	}

	type prKey struct{ url, name string }
	prIDs := make(map[prKey]uint)
	rows, err := conn.Query(r.ctx, `
		SELECT rp.id, rp.url, rp.name
		FROM release_pull_requests rp
		INNER JOIN tmp_release_prs tmp ON rp.url = tmp.url AND rp.name = tmp.name
		WHERE rp.deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("fetching resolved PR IDs: %w", err)
	}
	for rows.Next() {
		var id uint
		var prURL, prName string
		if err := rows.Scan(&id, &prURL, &prName); err != nil {
			rows.Close()
			return fmt.Errorf("scanning resolved PR ID: %w", err)
		}
		prIDs[prKey{prURL, prName}] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating resolved PR IDs: %w", err)
	}

	for _, tag := range tags {
		for i, pr := range tag.PullRequests {
			if id, ok := prIDs[prKey{pr.URL, pr.Name}]; ok {
				tag.PullRequests[i].ID = id
			}
		}
	}

	log.WithFields(log.Fields{
		"total":    len(allPRs),
		"upserted": upsertTag.RowsAffected(),
	}).Info("resolved pull requests across all tags")

	return nil
}

// bulkWriteReleaseTags inserts release tags and their associations
// (repositories, job runs, PR join table) using COPY + SQL on a single pgx
// connection. PRs must already exist in release_pull_requests (ensured by
// resolveAllPullRequests).
func (r *ReleaseLoader) bulkWriteReleaseTags(tags []*models.ReleaseTag) error {
	if len(tags) == 0 {
		return nil
	}

	sqlDB, err := r.db.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	tx, err := conn.Begin(r.ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(r.ctx) //nolint:errcheck

	if err := r.insertReleaseTags(tx, tags); err != nil {
		return err
	}
	if err := r.insertReleaseRepositories(tx, tags); err != nil {
		return err
	}
	if err := r.insertReleaseJobRuns(tx, tags); err != nil {
		return err
	}
	if err := r.insertReleaseTagPullRequests(tx, tags); err != nil {
		return err
	}

	if err := tx.Commit(r.ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	log.WithField("tags", len(tags)).Info("bulk-wrote release tags and associations via COPY")
	return nil
}

func (r *ReleaseLoader) insertReleaseTags(conn db.PgxSession, tags []*models.ReleaseTag) error {
	cleanup, err := db.CopyToTempTable(r.ctx, conn, "tmp_release_tags_insert", tags,
		[]db.TempColumn[*models.ReleaseTag]{
			{Name: "tag_name", Type: "text NOT NULL", Value: func(t **models.ReleaseTag) any { return (*t).ReleaseTag }},
			{Name: "release", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).Release }},
			{Name: "stream", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).Stream }},
			{Name: "architecture", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).Architecture }},
			{Name: "phase", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).Phase }},
			{Name: "forced", Type: "boolean NOT NULL DEFAULT false", Value: func(t **models.ReleaseTag) any { return (*t).Forced }},
			{Name: "release_time", Type: "timestamptz NOT NULL DEFAULT '0001-01-01'", Value: func(t **models.ReleaseTag) any { return (*t).ReleaseTime }},
			{Name: "previous_release_tag", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).PreviousReleaseTag }},
			{Name: "kubernetes_version", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).KubernetesVersion }},
			{Name: "current_os_version", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).CurrentOSVersion }},
			{Name: "previous_os_version", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).PreviousOSVersion }},
			{Name: "current_os_url", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).CurrentOSURL }},
			{Name: "previous_os_url", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).PreviousOSURL }},
			{Name: "os_diff_url", Type: "text NOT NULL DEFAULT ''", Value: func(t **models.ReleaseTag) any { return (*t).OSDiffURL }},
		},
	)
	defer cleanup()
	if err != nil {
		return err
	}

	_, err = conn.Exec(r.ctx, `
		INSERT INTO release_tags (
			release_tag, release, stream, architecture, phase, forced,
			release_time, previous_release_tag, kubernetes_version,
			current_os_version, previous_os_version, current_os_url,
			previous_os_url, os_diff_url, created_at, updated_at
		)
		SELECT
			tag_name, release, stream, architecture, phase, forced,
			release_time, previous_release_tag, kubernetes_version,
			current_os_version, previous_os_version, current_os_url,
			previous_os_url, os_diff_url, NOW(), NOW()
		FROM tmp_release_tags_insert
	`)
	return err
}

func (r *ReleaseLoader) insertReleaseRepositories(conn db.PgxSession, tags []*models.ReleaseTag) error {
	type repoRow struct {
		tagName string
		name    string
		head    string
		diffURL string
	}
	var repos []repoRow
	for _, t := range tags {
		for _, repo := range t.Repositories {
			repos = append(repos, repoRow{t.ReleaseTag, repo.Name, repo.Head, repo.DiffURL})
		}
	}
	cleanup, err := db.CopyToTempTable(r.ctx, conn, "tmp_release_repos", repos,
		[]db.TempColumn[repoRow]{
			{Name: "tag_name", Type: "text NOT NULL", Value: func(r *repoRow) any { return r.tagName }},
			{Name: "name", Type: "text NOT NULL DEFAULT ''", Value: func(r *repoRow) any { return r.name }},
			{Name: "head", Type: "text NOT NULL DEFAULT ''", Value: func(r *repoRow) any { return r.head }},
			{Name: "diff_url", Type: "text NOT NULL DEFAULT ''", Value: func(r *repoRow) any { return r.diffURL }},
		},
	)
	defer cleanup()
	if err != nil {
		return err
	}

	_, err = conn.Exec(r.ctx, `
		INSERT INTO release_repositories (release_tag_id, name, repository_head, diff_url, created_at, updated_at)
		SELECT rt.id, tmp.name, tmp.head, tmp.diff_url, NOW(), NOW()
		FROM tmp_release_repos tmp
		INNER JOIN release_tags rt ON rt.release_tag = tmp.tag_name AND rt.deleted_at IS NULL
	`)
	return err
}

func (r *ReleaseLoader) insertReleaseJobRuns(conn db.PgxSession, tags []*models.ReleaseTag) error {
	type jobRunRow struct {
		tagName        string
		prowJobRunID   uint
		jobName        string
		kind           string
		state          string
		transitionTime time.Time
		retries        int
		url            string
		upgradesFrom   string
		upgradesTo     string
		upgrade        bool
		labels         pq.StringArray
	}
	var jobRuns []jobRunRow
	for _, t := range tags {
		for _, jr := range t.JobRuns {
			jobRuns = append(jobRuns, jobRunRow{
				tagName: t.ReleaseTag, prowJobRunID: jr.Name,
				jobName: jr.JobName, kind: jr.Kind, state: jr.State,
				transitionTime: jr.TransitionTime, retries: jr.Retries,
				url: jr.URL, upgradesFrom: jr.UpgradesFrom,
				upgradesTo: jr.UpgradesTo, upgrade: jr.Upgrade,
				labels: jr.Labels,
			})
		}
	}
	cleanup, err := db.CopyToTempTable(r.ctx, conn, "tmp_release_job_runs", jobRuns,
		[]db.TempColumn[jobRunRow]{
			{Name: "tag_name", Type: "text NOT NULL", Value: func(jr *jobRunRow) any { return jr.tagName }},
			{Name: "prow_job_run_id", Type: "bigint NOT NULL", Value: func(jr *jobRunRow) any { return jr.prowJobRunID }},
			{Name: "job_name", Type: "text NOT NULL DEFAULT ''", Value: func(jr *jobRunRow) any { return jr.jobName }},
			{Name: "kind", Type: "text NOT NULL DEFAULT ''", Value: func(jr *jobRunRow) any { return jr.kind }},
			{Name: "state", Type: "text NOT NULL DEFAULT ''", Value: func(jr *jobRunRow) any { return jr.state }},
			{Name: "transition_time", Type: "timestamptz NOT NULL DEFAULT '0001-01-01'", Value: func(jr *jobRunRow) any { return jr.transitionTime }},
			{Name: "retries", Type: "integer NOT NULL DEFAULT 0", Value: func(jr *jobRunRow) any { return jr.retries }},
			{Name: "url", Type: "text NOT NULL DEFAULT ''", Value: func(jr *jobRunRow) any { return jr.url }},
			{Name: "upgrades_from", Type: "text NOT NULL DEFAULT ''", Value: func(jr *jobRunRow) any { return jr.upgradesFrom }},
			{Name: "upgrades_to", Type: "text NOT NULL DEFAULT ''", Value: func(jr *jobRunRow) any { return jr.upgradesTo }},
			{Name: "upgrade", Type: "boolean NOT NULL DEFAULT false", Value: func(jr *jobRunRow) any { return jr.upgrade }},
			{Name: "labels", Type: "text[]", Value: func(jr *jobRunRow) any { return jr.labels }},
		},
	)
	defer cleanup()
	if err != nil {
		return err
	}

	_, err = conn.Exec(r.ctx, `
		INSERT INTO release_job_runs (
			release_tag_id, prow_job_run_id, job_name, kind, state,
			transition_time, retries, url, upgrades_from, upgrades_to,
			upgrade, labels, created_at, updated_at
		)
		SELECT
			rt.id, tmp.prow_job_run_id, tmp.job_name, tmp.kind, tmp.state,
			tmp.transition_time, tmp.retries, tmp.url, tmp.upgrades_from,
			tmp.upgrades_to, tmp.upgrade, tmp.labels, NOW(), NOW()
		FROM tmp_release_job_runs tmp
		INNER JOIN release_tags rt ON rt.release_tag = tmp.tag_name AND rt.deleted_at IS NULL
		ON CONFLICT (prow_job_run_id) DO UPDATE SET
			release_tag_id  = EXCLUDED.release_tag_id,
			job_name        = EXCLUDED.job_name,
			kind            = EXCLUDED.kind,
			state           = EXCLUDED.state,
			transition_time = EXCLUDED.transition_time,
			retries         = EXCLUDED.retries,
			url             = EXCLUDED.url,
			upgrades_from   = EXCLUDED.upgrades_from,
			upgrades_to     = EXCLUDED.upgrades_to,
			upgrade         = EXCLUDED.upgrade,
			labels          = EXCLUDED.labels,
			updated_at      = NOW()
		WHERE release_job_runs.release_tag_id  IS DISTINCT FROM EXCLUDED.release_tag_id
		   OR release_job_runs.job_name        IS DISTINCT FROM EXCLUDED.job_name
		   OR release_job_runs.kind            IS DISTINCT FROM EXCLUDED.kind
		   OR release_job_runs.state           IS DISTINCT FROM EXCLUDED.state
		   OR release_job_runs.transition_time IS DISTINCT FROM EXCLUDED.transition_time
		   OR release_job_runs.retries         IS DISTINCT FROM EXCLUDED.retries
		   OR release_job_runs.url             IS DISTINCT FROM EXCLUDED.url
		   OR release_job_runs.upgrades_from   IS DISTINCT FROM EXCLUDED.upgrades_from
		   OR release_job_runs.upgrades_to     IS DISTINCT FROM EXCLUDED.upgrades_to
		   OR release_job_runs.upgrade         IS DISTINCT FROM EXCLUDED.upgrade
		   OR release_job_runs.labels          IS DISTINCT FROM EXCLUDED.labels
	`)
	return err
}

func (r *ReleaseLoader) insertReleaseTagPullRequests(conn db.PgxSession, tags []*models.ReleaseTag) error {
	type joinRow struct {
		tagName string
		prID    uint
	}
	var joins []joinRow
	for _, t := range tags {
		for _, pr := range t.PullRequests {
			if pr.ID != 0 {
				joins = append(joins, joinRow{t.ReleaseTag, pr.ID})
			}
		}
	}
	cleanup, err := db.CopyToTempTable(r.ctx, conn, "tmp_release_tag_prs", joins,
		[]db.TempColumn[joinRow]{
			{Name: "tag_name", Type: "text NOT NULL", Value: func(j *joinRow) any { return j.tagName }},
			{Name: "pr_id", Type: "bigint NOT NULL", Value: func(j *joinRow) any { return j.prID }},
		},
	)
	defer cleanup()
	if err != nil {
		return err
	}

	_, err = conn.Exec(r.ctx, `
		INSERT INTO release_tag_pull_requests (release_tag_id, release_pull_request_id)
		SELECT rt.id, tmp.pr_id
		FROM tmp_release_tag_prs tmp
		INNER JOIN release_tags rt ON rt.release_tag = tmp.tag_name AND rt.deleted_at IS NULL
		ON CONFLICT (release_tag_id, release_pull_request_id) DO NOTHING
	`)
	return err
}

func (r *ReleaseLoader) fetchReleaseDetails(rs ReleaseStream, tag ReleaseTag) *ReleaseDetails {
	releaseDetails := ReleaseDetails{}
	rcURL := rs.buildDetailsURL(tag.Name)

	resp, err := r.httpClient.Get(rcURL)
	if err != nil {
		log.WithError(err).Errorf("error fetching release details from %s", rcURL)
		return nil
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&releaseDetails); err != nil {
		log.WithError(err).Errorf("error decoding release details JSON from %s", rcURL)
		return nil
	}

	return &releaseDetails
}

func (r *ReleaseLoader) fetchReleaseTags(rs ReleaseStream) []ReleaseTag {
	uri := rs.buildTagsURL()
	resp, err := r.httpClient.Get(uri)
	if err != nil {
		log.WithError(err).Errorf("failed to connect to release controller at %s", uri)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		log.Warnf("release controller returned non-200 error code for %s: %d %s", uri, resp.StatusCode, resp.Status)
		return nil
	}

	tags := ReleaseTags{}
	err = json.NewDecoder(resp.Body).Decode(&tags)
	defer resp.Body.Close()
	if err != nil {
		log.Errorf("couldn't decode json: %v", err)
		return nil
	}
	return tags.Tags
}

func (r *ReleaseLoader) releaseDetailsToDB(rs ReleaseStream, tag ReleaseTag, details ReleaseDetails) *models.ReleaseTag {
	release := models.ReleaseTag{
		Release:      rs.Release.Release,
		Stream:       rs.Stream,
		Architecture: rs.Architecture,
		ReleaseTag:   details.Name,
		Phase:        tag.Phase,
	}

	dateTime := regexp.MustCompile(`.*([0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{6})`)
	match := dateTime.FindStringSubmatch(tag.Name)
	if len(match) > 1 {
		t, err := time.Parse("2006-01-02-150405", match[1])
		if err == nil {
			release.ReleaseTime = t
		}
	}

	if len(details.ChangeLog) == 0 {
		return nil // changelog not available yet
	}

	if len(details.ChangeLogJSON.Components) > 0 {
		jsonChangeLog := parseChangeLogJSON(tag.Name, details.ChangeLogJSON)

		release.KubernetesVersion = jsonChangeLog.KubernetesVersion
		release.CurrentOSURL = jsonChangeLog.CurrentOSURL
		release.CurrentOSVersion = jsonChangeLog.CurrentOSVersion
		release.PreviousOSURL = jsonChangeLog.PreviousOSURL
		release.PreviousOSVersion = jsonChangeLog.PreviousOSVersion
		release.OSDiffURL = jsonChangeLog.OSDiffURL

		release.PreviousReleaseTag = jsonChangeLog.PreviousReleaseTag
		release.Repositories = jsonChangeLog.Repositories
		release.PullRequests = jsonChangeLog.PullRequests

	} else {
		changelog := NewChangelog(tag.Name, string(details.ChangeLog))
		release.KubernetesVersion = changelog.KubernetesVersion()
		release.CurrentOSURL, release.CurrentOSVersion, release.PreviousOSURL, release.PreviousOSVersion, release.OSDiffURL = changelog.CoreOSVersion()
		release.PreviousReleaseTag = changelog.PreviousReleaseTag()
		release.Repositories = changelog.Repositories()
		release.PullRequests = changelog.PullRequests()
	}
	release.JobRuns = r.buildJobRuns(details)

	// set forced flag
	failedBlocking := false

	for _, jRun := range release.JobRuns {
		if jRun.State == failed {
			if jRun.Kind == "Blocking" {
				failedBlocking = true
				break
			}
		}
	}

	switch release.Phase {
	case "Accepted":
		release.Forced = failedBlocking
	case "Rejected":
		release.Forced = !failedBlocking
	}

	return &release
}

func parseChangeLogJSON(releaseTag string, changeLogJSON ChangeLog) models.ReleaseTag {
	releaseChangeLogJSON := models.ReleaseTag{}

	releaseChangeLogJSON.PreviousReleaseTag = changeLogJSON.From.Name

	for _, c := range changeLogJSON.Components {
		if c.Name == "Kubernetes" {
			releaseChangeLogJSON.KubernetesVersion = c.Version
		} else if strings.Contains(c.Name, "CoreOS") {
			releaseChangeLogJSON.CurrentOSVersion = c.Version
			releaseChangeLogJSON.CurrentOSURL = c.VersionURL
			releaseChangeLogJSON.PreviousOSURL = c.FromURL
			releaseChangeLogJSON.PreviousOSVersion = c.From
			releaseChangeLogJSON.OSDiffURL = c.DiffURL
		}
	}

	type prlocator struct {
		name string
		url  string
	}

	releaseRepoRows := make([]models.ReleaseRepository, 0)
	releasePRRows := make(map[prlocator]models.ReleasePullRequest)
	for _, ui := range changeLogJSON.UpdatedImages {

		releaseRepoRow := models.ReleaseRepository{
			Name:    ui.Name,
			Head:    ui.Path,
			DiffURL: ui.FullChangeLog,
		}

		releaseRepoRows = append(releaseRepoRows, releaseRepoRow)

		for _, commit := range ui.Commits {
			releasePRRow := models.ReleasePullRequest{
				Name:          ui.Name,
				Description:   commit.Subject,
				URL:           commit.PullURL,
				PullRequestID: fmt.Sprintf("%d", commit.PullID),
			}

			// saves the last one..
			for _, value := range commit.Issues {
				releasePRRow.BugURL = value
			}

			for _, value := range commit.Bugs {
				releasePRRow.BugURL = value
			}

			prl := prlocator{
				url:  releasePRRow.URL,
				name: releasePRRow.Name,
			}
			if _, ok := releasePRRows[prl]; ok {
				log.Warningf("duplicate PR in %q: %q, %q", releaseTag, releasePRRow.URL, releasePRRow.Name)
			} else {
				releasePRRows[prl] = releasePRRow
			}
		}

	}

	releaseChangeLogJSON.Repositories = releaseRepoRows

	releasePullRequestResult := make([]models.ReleasePullRequest, 0)
	items := 0
	for _, v := range releasePRRows {
		// We had a case of a release payload changelog that contained 235,000 pull requests. Sippy got stuck on it
		// so this check is here to prevent something like that from ever happening again.  2,500 seems like a very
		// reasonable upper bound.
		if items > 2500 {
			log.Warningf("%q had more than 2,500 PR's! Ignoring the rest to protect ourself.", releaseTag)
			break
		}
		releasePullRequestResult = append(releasePullRequestResult, v)
		items++
	}

	releaseChangeLogJSON.PullRequests = releasePullRequestResult

	return releaseChangeLogJSON
}

// buildJobRuns converts release details into ReleaseJobRun models without
// fetching labels from BigQuery. Labels are applied separately by
// applyBulkLabels during Load().
func (r *ReleaseLoader) buildJobRuns(details ReleaseDetails) []models.ReleaseJobRun {
	results := make(map[uint]models.ReleaseJobRun)

	recordResultsFrom := func(element, resultKind string) {
		if jobs, ok := details.Results[element]; ok {
			for platform, jobResult := range jobs {
				id, err := idFromURL(jobResult.URL)
				if id == 0 || err != nil {
					log.WithFields(log.Fields{
						"id":         id,
						"releaseTag": details.Name,
						"url":        jobResult.URL,
						"platform":   platform,
						"error":      err,
					}).Warningf("invalid ID or missing URL for job")
					continue
				}

				results[id] = models.ReleaseJobRun{
					Name:           id,
					JobName:        platform,
					Kind:           resultKind,
					State:          jobResult.State,
					URL:            jobResult.URL,
					Retries:        jobResult.Retries,
					TransitionTime: jobResult.TransitionTime,
				}
			}
		}
	}
	recordResultsFrom("blockingJobs", "Blocking")
	recordResultsFrom("informingJobs", "Informing")

	for _, upgrade := range append(details.UpgradesTo, details.UpgradesFrom...) {
		for _, run := range upgrade.History {
			id, err := idFromURL(run.URL)
			if id == 0 || err != nil {
				log.WithFields(log.Fields{
					"id":         id,
					"releaseTag": details.Name,
					"url":        run.URL,
					"error":      err,
				}).Warningf("invalid ID or missing URL for job")
				continue
			}

			if result, ok := results[id]; ok {
				result.Upgrade = true
				result.UpgradesFrom = upgrade.From
				result.UpgradesTo = upgrade.To
				results[id] = result
			}
		}
	}

	rows := make([]models.ReleaseJobRun, 0, len(results))
	for _, result := range results {
		rows = append(rows, result)
	}
	return rows
}

// applyBulkLabels fetches labels from BigQuery for all job runs across all
// tags in a single query, then distributes them back.
func (r *ReleaseLoader) applyBulkLabels(tags []*models.ReleaseTag) {
	if r.bqClient == nil || len(tags) == 0 {
		return
	}

	var buildIDs []string
	var earliestTime time.Time
	for _, tag := range tags {
		for _, jr := range tag.JobRuns {
			if jr.Name != 0 {
				buildIDs = append(buildIDs, strconv.FormatUint(uint64(jr.Name), 10))
			}
		}
		if !tag.ReleaseTime.IsZero() && (earliestTime.IsZero() || tag.ReleaseTime.Before(earliestTime)) {
			earliestTime = tag.ReleaseTime
		}
	}
	if len(buildIDs) == 0 || earliestTime.IsZero() {
		return
	}

	labelsByBuildID, err := prowloader.GatherLabelsFromBQ(r.ctx, r.bqClient, buildIDs, earliestTime)
	if err != nil {
		log.WithError(err).Warning("failed to fetch bulk labels from BigQuery")
		r.errors = append(r.errors, fmt.Errorf("GatherLabelsFromBQ: %w", err))
		return
	}

	for _, tag := range tags {
		for i := range tag.JobRuns {
			buildID := strconv.FormatUint(uint64(tag.JobRuns[i].Name), 10)
			if labels, ok := labelsByBuildID[buildID]; ok {
				tag.JobRuns[i].Labels = labels
			}
		}
	}
}

func idFromURL(prowURL string) (uint, error) {
	if prowURL == "" {
		return 0, fmt.Errorf("prowURL should not be blank")
	}

	parsed, err := url.Parse(prowURL)
	if err != nil {
		return 0, err
	}

	base := path.Base(parsed.Path)
	prowID, err := strconv.ParseUint(base, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(prowID), nil
}

// extractBuildIDFromURL extracts the build ID from a prow job URL
// e.g., https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1234567890
// returns "1234567890"
func extractBuildIDFromURL(prowURL string) string {
	if prowURL == "" {
		return ""
	}

	parsed, err := url.Parse(prowURL)
	if err != nil {
		return ""
	}

	return path.Base(parsed.Path)
}

func (rs *ReleaseStream) buildTagsURL() string {
	return fmt.Sprintf("%s/%s/tags", rs.baseReleaseStreamURL(), rs.Name)
}

func (rs *ReleaseStream) buildDetailsURL(tag string) string {
	return fmt.Sprintf("%s/%s/release/%s", rs.baseReleaseStreamURL(), rs.Name, tag)
}

func (rs *ReleaseStream) baseReleaseStreamURL() string {
	return fmt.Sprintf("https://%s/api/v1/releasestream", rs.Domain)
}

// buildReleaseStreams builds relevant release streams for specified releases that belong to the project.
func buildReleaseStreams(releases map[string]v1.Release, architectures []string, project PayloadProject) []ReleaseStream {
	releaseStreams := make([]ReleaseStream, 0, len(releases)*len(project.GetStreams())*len(architectures))
	for release, config := range releases {
		if project.IsProjectRelease(config) {
			for _, stream := range project.GetStreams() {
				for _, arch := range architectures {
					if name := project.FullReleaseStream(release, stream, arch); name != "" {
						releaseStreams = append(releaseStreams, ReleaseStream{
							Name:         name,
							Release:      config,
							Stream:       stream,
							Architecture: arch,
							Domain:       project.GetRcDomain(arch),
						})
					}
				}
			}
		}
	}
	return releaseStreams
}
