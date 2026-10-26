package verify

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"k8s.io/apimachinery/pkg/util/sets"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/apis/prow"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/releaseoverride"
)

const (
	bqProgressLogInterval    = 1000
	bqJobProgressLogInterval = 30 * time.Second
)

func logBQProgress(rows int, start, end time.Time) {
	if rows%bqProgressLogInterval != 0 {
		return
	}
	log.WithFields(log.Fields{
		"rows":  rows,
		"start": start,
		"end":   end,
	}).Info("read BigQuery Prow jobs")
}

func bigQueryJobState(state bigquery.State) string {
	switch state {
	case bigquery.Pending:
		return "pending"
	case bigquery.Running:
		return "running"
	case bigquery.Done:
		return "done"
	default:
		return "unknown"
	}
}

func waitForBigQueryJob(ctx context.Context, submittedAt time.Time, statusFunc func(context.Context) (*bigquery.JobStatus, error)) error {
	logBQJobStatus := func(status *bigquery.JobStatus) {
		log.WithFields(log.Fields{
			"elapsed": time.Since(submittedAt),
			"state":   bigQueryJobState(status.State),
		}).Info("BigQuery Prow jobs query status")
	}

	for {
		status, err := statusFunc(ctx)
		if err != nil {
			return fmt.Errorf("checking BigQuery Prow jobs query status: %w", err)
		}
		logBQJobStatus(status)
		if status.Done() {
			if err := status.Err(); err != nil {
				return fmt.Errorf("BigQuery Prow jobs query failed: %w", err)
			}
			log.WithField("elapsed", time.Since(submittedAt)).Info("BigQuery Prow jobs query complete")
			return nil
		}

		timer := time.NewTimer(bqJobProgressLogInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type BQCompletenessVerifier struct {
	PostgreSQL                  ProwJobRunReader
	BigQuery                    BigQueryReader
	BigQueryInitializationError error
	Config                      *v1config.SippyConfig
	SyntheticReleaseOverrides   *releaseoverride.SyntheticReleaseOverrides
}

type postgresRun struct {
	Release string
	Seen    bool
}

func (v *BQCompletenessVerifier) Verify(ctx context.Context, scope Scope) Result {
	result := Result{}
	if v.BigQueryInitializationError != nil {
		for _, release := range scope.Releases {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, v.BigQueryInitializationError))
		}
		return result
	}
	if v.BigQuery == nil {
		err := fmt.Errorf("BigQuery client is not initialized")
		for _, release := range scope.Releases {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, err))
		}
		return result
	}

	start := scope.Date.In(time.UTC)
	end := scope.Date.AddDays(1).In(time.UTC)
	postgresRuns, expectedRows, availableReleases, postgresResult := v.readPostgresRuns(ctx, scope, start, end)
	result.Summaries = append(result.Summaries, postgresResult.Summaries...)

	attributor := prowloader.NewReleaseAttributor(scope.Releases, v.Config, v.SyntheticReleaseOverrides)
	seenBQIDs := make(map[string]map[BuildID]struct{}, len(scope.Releases))
	malformedSets := make(map[string]sets.Set[string], len(scope.Releases))
	missingInPostgres := make(map[string]map[BuildID]struct{}, len(scope.Releases))
	err := v.BigQuery.ForEachProwJob(ctx, start, end, func(job BQJob) error {
		return v.processBQJob(job, attributor, availableReleases, postgresRuns, seenBQIDs, malformedSets, missingInPostgres)
	})
	if err != nil {
		result.Summaries = nil
		for _, release := range scope.Releases {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, err))
		}
		return result
	}

	appendBQResults(&result, scope, scope.Releases, availableReleases, expectedRows, postgresRuns, seenBQIDs, malformedSets, missingInPostgres)
	return result
}

func (v *BQCompletenessVerifier) readPostgresRuns(ctx context.Context, scope Scope, start, end time.Time) (map[BuildID]postgresRun, map[string]int, sets.Set[string], Result) {
	postgresRuns := make(map[BuildID]postgresRun)
	expectedRows := make(map[string]int, len(scope.Releases))
	availableReleases := sets.New[string]()
	result := Result{}
	for _, release := range scope.Releases {
		ids, err := v.PostgreSQL.ProwJobRunIDs(ctx, release, start, end)
		if err != nil {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, err))
			continue
		}
		availableReleases.Insert(release)
		expectedRows[release] = len(ids)
		for id := range ids {
			postgresRuns[id] = postgresRun{Release: release}
		}
	}
	return postgresRuns, expectedRows, availableReleases, result
}

func (v *BQCompletenessVerifier) processBQJob(job BQJob, attributor *prowloader.ReleaseAttributor, availableReleases sets.Set[string], postgresRuns map[BuildID]postgresRun, seenBQIDs map[string]map[BuildID]struct{}, malformedSets map[string]sets.Set[string], missingInPostgres map[string]map[BuildID]struct{}) error {
	value := strings.TrimSpace(job.BuildID)
	id, parseErr := strconv.ParseUint(value, 10, 64)
	release := ""
	if parseErr == nil {
		// Generated config can drop a job after its run was imported.
		if run, ok := postgresRuns[BuildID(id)]; ok {
			release = run.Release
		}
	}
	if release == "" {
		pj := &prow.ProwJob{
			Annotations: job.Annotations,
			Spec:        prow.ProwJobSpec{Job: job.JobName},
		}
		if job.HasRefs {
			pj.Spec.Refs = &prow.Refs{}
		}
		release = attributor.Match(pj)
	}
	if release == "" || !availableReleases.Has(release) {
		return nil
	}
	if parseErr != nil {
		if malformedSets[release] == nil {
			malformedSets[release] = sets.New[string]()
		}
		malformedSets[release].Insert(value)
		return nil
	}
	if seenBQIDs[release] == nil {
		seenBQIDs[release] = make(map[BuildID]struct{})
	}
	if _, seen := seenBQIDs[release][BuildID(id)]; seen {
		return nil
	}
	seenBQIDs[release][BuildID(id)] = struct{}{}
	if run, ok := postgresRuns[BuildID(id)]; ok {
		run.Seen = true
		postgresRuns[BuildID(id)] = run
		return nil
	}
	if missingInPostgres[release] == nil {
		missingInPostgres[release] = make(map[BuildID]struct{})
	}
	missingInPostgres[release][BuildID(id)] = struct{}{}
	return nil
}

func appendBQResults(result *Result, scope Scope, releases []string, availableReleases sets.Set[string], expectedRows map[string]int, postgresRuns map[BuildID]postgresRun, seenBQIDs map[string]map[BuildID]struct{}, malformedSets map[string]sets.Set[string], missingInPostgres map[string]map[BuildID]struct{}) {
	for _, release := range releases {
		if !availableReleases.Has(release) {
			continue
		}
		discrepancies := bqDiscrepancies(scope, release, postgresRuns, malformedSets, missingInPostgres)
		result.Summaries = append(result.Summaries, summary(CheckBQCompleteness, release, scope.Date, len(seenBQIDs[release]), expectedRows[release], len(discrepancies)))
		result.Discrepancies = append(result.Discrepancies, discrepancies...)
	}
}

func bqDiscrepancies(scope Scope, release string, postgresRuns map[BuildID]postgresRun, malformedSets map[string]sets.Set[string], missingInPostgres map[string]map[BuildID]struct{}) []Discrepancy {
	discrepancies := make([]Discrepancy, 0)
	for _, value := range sets.List(malformedSets[release]) {
		kind := "malformed-build-id"
		detail := "BigQuery build ID is not an unsigned integer"
		if value == "" {
			kind = "missing-build-id"
			detail = "BigQuery build ID is blank"
		}
		discrepancies = append(discrepancies, Discrepancy{
			Check: CheckBQCompleteness, Release: release, Date: scope.Date,
			Kind: kind, Key: value, Detail: detail,
		})
	}
	for _, id := range sortedBuildIDs(missingInPostgres[release]) {
		discrepancies = append(discrepancies, Discrepancy{
			Check: CheckBQCompleteness, Release: release, Date: scope.Date,
			Kind: "missing-in-postgres", Key: fmt.Sprint(uint64(id)), Expected: "present", Actual: "missing",
		})
	}
	missingInBigQuery := make([]BuildID, 0)
	for id, run := range postgresRuns {
		if run.Release == release && !run.Seen {
			missingInBigQuery = append(missingInBigQuery, id)
		}
	}
	sort.Slice(missingInBigQuery, func(i, j int) bool { return missingInBigQuery[i] < missingInBigQuery[j] })
	for _, id := range missingInBigQuery {
		discrepancies = append(discrepancies, Discrepancy{
			Check: CheckBQCompleteness, Release: release, Date: scope.Date,
			Kind: "missing-in-bigquery", Key: fmt.Sprint(uint64(id)), Expected: "present", Actual: "missing",
		})
	}
	return discrepancies
}

func (p *PostgreSQL) ProwJobRunIDs(ctx context.Context, release string, start, end time.Time) (map[BuildID]struct{}, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	type row struct {
		ID uint64
	}
	var rows []row
	err := p.dbc.DB.WithContext(ctx).Raw(`
		SELECT id
		FROM prow_job_runs
		WHERE prow_job_release = ? AND timestamp >= ? AND timestamp < ? AND deleted_at IS NULL
		ORDER BY id
	`, release, start, end).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("querying PostgreSQL Prow job runs for release %s: %w", release, err)
	}
	result := make(map[BuildID]struct{}, len(rows))
	for _, row := range rows {
		result[BuildID(row.ID)] = struct{}{}
	}
	return result, nil
}

type BigQuery struct {
	client *bqcachedclient.Client
}

func NewBigQuery(client *bqcachedclient.Client) *BigQuery {
	return &BigQuery{client: client}
}

var bigQueryIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ForEachProwJob reads one date range from BigQuery and processes each job
// without retaining the complete result set in memory.
func (b *BigQuery) ForEachProwJob(ctx context.Context, start, end time.Time, handler ProwJobHandler) error {
	if b == nil || b.client == nil || b.client.BQ == nil {
		return fmt.Errorf("BigQuery client is not initialized")
	}
	project := b.client.BQ.Project()
	dataset := b.client.Dataset
	if !bigQueryIdentifier.MatchString(project) || !bigQueryIdentifier.MatchString(dataset) {
		return fmt.Errorf("invalid BigQuery project or dataset identifier")
	}
	query := b.client.Query(ctx, bqlabel.VerifyProwJobs, fmt.Sprintf(`
		SELECT
			prowjob_job_name,
			IFNULL(CAST(prowjob_build_id AS STRING), '') AS prowjob_build_id,
			prowjob_annotations,
			IFNULL(CAST(pr_number AS STRING), '') AS pr_number
		FROM %s
		WHERE TIMESTAMP(prowjob_start) >= @start
		  AND TIMESTAMP(prowjob_start) < @end
		  AND prowjob_url IS NOT NULL
		  AND prowjob_state NOT IN ('pending', 'triggered')
		ORDER BY TIMESTAMP(prowjob_start), prowjob_build_id
	`, "`"+project+"."+dataset+".jobs`"))
	query.Parameters = []bigquery.QueryParameter{
		{Name: "start", Value: start},
		{Name: "end", Value: end},
	}
	submittedAt := time.Now()
	job, err := query.Run(ctx)
	if err != nil {
		return fmt.Errorf("starting BigQuery Prow jobs query: %w", err)
	}
	if err := waitForBigQueryJob(ctx, submittedAt, job.Status); err != nil {
		return err
	}
	iteratorRows, err := job.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading BigQuery Prow jobs query results: %w", err)
	}
	type bqRow struct {
		JobName     string   `bigquery:"prowjob_job_name"`
		BuildID     string   `bigquery:"prowjob_build_id"`
		Annotations []string `bigquery:"prowjob_annotations"`
		PRNumber    string   `bigquery:"pr_number"`
	}
	rowsRead := 0
	for {
		var row bqRow
		err := iteratorRows.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("reading BigQuery Prow jobs: %w", err)
		}
		rowsRead++
		logBQProgress(rowsRead, start, end)
		annotations := make(map[string]string, len(row.Annotations))
		for _, annotation := range row.Annotations {
			parts := strings.SplitN(annotation, "=", 2)
			if len(parts) == 2 {
				annotations[parts[0]] = parts[1]
			}
		}
		if err := handler(BQJob{
			BuildID: row.BuildID, JobName: row.JobName, Annotations: annotations, HasRefs: row.PRNumber != "",
		}); err != nil {
			return fmt.Errorf("processing BigQuery Prow job: %w", err)
		}
	}
	return nil
}

func CompareBuildIDs(release string, date civil.Date, bqIDs, postgresIDs map[BuildID]struct{}, malformed []string) (Summary, []Discrepancy) {
	discrepancies := make([]Discrepancy, 0)
	malformedSet := sets.New[string]()
	for _, value := range malformed {
		malformedSet.Insert(strings.TrimSpace(value))
	}
	for _, value := range sets.List(malformedSet) {
		kind := "malformed-build-id"
		detail := "BigQuery build ID is not an unsigned integer"
		if value == "" {
			kind = "missing-build-id"
			detail = "BigQuery build ID is blank"
		}
		discrepancies = append(discrepancies, Discrepancy{
			Check: CheckBQCompleteness, Release: release, Date: date,
			Kind: kind, Key: value, Detail: detail,
		})
	}
	for _, id := range sortedBuildIDs(bqIDs) {
		if _, ok := postgresIDs[id]; !ok {
			discrepancies = append(discrepancies, Discrepancy{
				Check: CheckBQCompleteness, Release: release, Date: date,
				Kind: "missing-in-postgres", Key: fmt.Sprint(uint64(id)), Expected: "present", Actual: "missing",
			})
		}
	}
	for _, id := range sortedBuildIDs(postgresIDs) {
		if _, ok := bqIDs[id]; !ok {
			discrepancies = append(discrepancies, Discrepancy{
				Check: CheckBQCompleteness, Release: release, Date: date,
				Kind: "missing-in-bigquery", Key: fmt.Sprint(uint64(id)), Expected: "present", Actual: "missing",
			})
		}
	}
	return summary(CheckBQCompleteness, release, date, len(bqIDs), len(postgresIDs), len(discrepancies)), discrepancies
}

func sortedBuildIDs(values map[BuildID]struct{}) []BuildID {
	ids := make([]BuildID, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
