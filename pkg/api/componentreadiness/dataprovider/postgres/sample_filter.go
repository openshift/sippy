package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"
)

// buildSamplePullRequestAndPayloadRunFilter builds the pull request and payload
// tag predicates shared by sample reports and test details. The time bounds
// constrain the partitioned pull request run lookup. EXISTS avoids multiplying
// test results when a run has multiple associations. The caller must provide
// the prow_job_runs alias pjr.
func buildSamplePullRequestAndPayloadRunFilter(release reqopts.Release, start, end time.Time) (string, []any, error) {
	var clauses []string
	var args []any

	if pr := release.PullRequestOptions; pr != nil {
		number, err := strconv.Atoi(pr.PRNumber)
		if err != nil {
			return "", nil, fmt.Errorf("invalid sample pull request number %q: %w", pr.PRNumber, err)
		}
		clauses = append(clauses, `EXISTS (
			SELECT 1
			FROM prow_job_run_prow_pull_requests pr_run
			JOIN prow_pull_requests pr ON pr.id = pr_run.prow_pull_request_id
			WHERE pr_run.prow_job_run_id = pjr.id
				AND pr_run.prow_job_run_release = pjr.prow_job_release
				AND pr_run.prow_job_run_timestamp = pjr.timestamp
				AND pr.org = ? AND pr.repo = ? AND pr.number = ?
				AND pr_run.prow_job_run_timestamp >= ? AND pr_run.prow_job_run_timestamp < ?
				AND pr.deleted_at IS NULL
		)`)
		args = append(args, pr.Org, pr.Repo, number, start, end)
	}

	if payload := release.PayloadOptions; payload != nil {
		clauses = append(clauses, `EXISTS (
			SELECT 1
			FROM release_job_runs payload_run
			JOIN release_tags rt ON rt.id = payload_run.release_tag_id
			WHERE payload_run.prow_job_run_id = pjr.id
				AND rt.release_tag = ANY(?)
				AND payload_run.deleted_at IS NULL AND rt.deleted_at IS NULL
		)`)
		args = append(args, pq.Array(payload.Tags))
	}

	return strings.Join(clauses, " AND "), args, nil
}

// findSampleReleases returns the releases containing runs selected by the
// sample's pull request or payload filter. Pull-request releases bound requests
// with both filters because the run predicate also intersects payload tags. It
// returns nil when neither filter is set; an empty non-nil slice means the
// filter matched no runs.
func findSampleReleases(ctx context.Context, db *gorm.DB, release reqopts.Release, start, end time.Time) ([]string, error) {
	if pr := release.PullRequestOptions; pr != nil {
		return findSamplePullRequestReleases(ctx, db, pr, start, end)
	}
	if payload := release.PayloadOptions; payload != nil {
		return findSamplePayloadReleases(ctx, db, payload, start, end)
	}
	return nil, nil
}

// findSamplePullRequestReleases finds releases with runs associated with the
// selected pull request in the requested sample window.
func findSamplePullRequestReleases(
	ctx context.Context,
	db *gorm.DB,
	pr *reqopts.PullRequest,
	start, end time.Time,
) ([]string, error) {
	number, err := strconv.Atoi(pr.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid sample pull request number %q: %w", pr.PRNumber, err)
	}

	type pullRequestReleaseRow struct {
		Release string `gorm:"column:prow_job_run_release"`
	}
	var releaseRows []pullRequestReleaseRow
	err = db.WithContext(ctx).Raw(`
		SELECT DISTINCT pr_run.prow_job_run_release
		FROM prow_job_run_prow_pull_requests pr_run
		JOIN prow_pull_requests pr ON pr.id = pr_run.prow_pull_request_id
		WHERE pr.org = ? AND pr.repo = ? AND pr.number = ?
			AND pr_run.prow_job_run_timestamp >= ? AND pr_run.prow_job_run_timestamp < ?
			AND pr.deleted_at IS NULL`,
		pr.Org, pr.Repo, number, start, end).Scan(&releaseRows).Error
	if err != nil {
		return nil, fmt.Errorf("finding releases for sample pull request: %w", err)
	}
	releases := make([]string, 0, len(releaseRows))
	for _, row := range releaseRows {
		releases = append(releases, row.Release)
	}

	return releases, nil
}

// findSamplePayloadReleases discovers authoritative Prow releases for runs
// associated with the selected payload tags in the sample window. The ID map
// supplies partition keys for locating each Prow run; release and timestamp
// values come from prow_job_runs.
func findSamplePayloadReleases(
	ctx context.Context,
	db *gorm.DB,
	payload *reqopts.Payload,
	start, end time.Time,
) ([]string, error) {

	type payloadRunLookup struct {
		ID             int64          `gorm:"column:id"`
		ProwJobRelease sql.NullString `gorm:"column:prow_job_release"`
		Timestamp      sql.NullTime   `gorm:"column:timestamp"`
	}
	var runLookups []payloadRunLookup
	err := db.WithContext(ctx).Raw(`
		SELECT DISTINCT payload_run.prow_job_run_id AS id,
			run_map.prow_job_release, run_map.timestamp
		FROM release_job_runs payload_run
		JOIN release_tags rt ON rt.id = payload_run.release_tag_id
		LEFT JOIN prow_job_run_id_map run_map ON run_map.id = payload_run.prow_job_run_id
		WHERE rt.release_tag = ANY(?)
			AND payload_run.deleted_at IS NULL
			AND rt.deleted_at IS NULL`,
		pq.Array(payload.Tags)).Scan(&runLookups).Error
	if err != nil {
		return nil, fmt.Errorf("finding Prow run lookup keys for sample payload tags: %w", err)
	}

	if len(runLookups) == 0 {
		return []string{}, nil
	}

	const maxLookupKeysPerQuery = 5000
	releases := sets.New[string]()
	mappedLookups := make([]payloadRunLookup, 0, len(runLookups))
	uniqueRunIDs := sets.New[int64]()
	var runIDs []int64
	for _, lookup := range runLookups {
		if uniqueRunIDs.Has(lookup.ID) {
			continue
		}
		uniqueRunIDs.Insert(lookup.ID)
		runIDs = append(runIDs, lookup.ID)
		if lookup.ProwJobRelease.Valid && lookup.Timestamp.Valid {
			mappedLookups = append(mappedLookups, lookup)
		}
	}

	foundRunIDs := sets.New[int64]()
	for offset := 0; offset < len(mappedLookups); offset += maxLookupKeysPerQuery {
		limit := min(offset+maxLookupKeysPerQuery, len(mappedLookups))
		lookupBatch := mappedLookups[offset:limit]
		valuePlaceholders := make([]string, len(lookupBatch))
		args := make([]any, 0, len(lookupBatch)*3)
		for i, lookup := range lookupBatch {
			valuePlaceholders[i] = "(CAST(? AS BIGINT), ?, CAST(? AS TIMESTAMPTZ))"
			args = append(args, lookup.ID, lookup.ProwJobRelease.String, lookup.Timestamp.Time)
		}

		var releaseRows []struct {
			ID        int64     `gorm:"column:id"`
			Release   string    `gorm:"column:prow_job_run_release"`
			Timestamp time.Time `gorm:"column:timestamp"`
		}
		query := `
			SELECT DISTINCT authoritative_run.id,
				authoritative_run.prow_job_release AS prow_job_run_release,
				authoritative_run.timestamp
			FROM (VALUES ` + strings.Join(valuePlaceholders, ",") + `) AS run_lookup(id, prow_job_release, run_timestamp)
			JOIN LATERAL (
				SELECT run_row.id, run_row.prow_job_release, run_row.timestamp
				FROM prow_job_runs run_row
				WHERE run_row.id = run_lookup.id
					AND run_row.prow_job_release = run_lookup.prow_job_release
					AND run_row.timestamp = run_lookup.run_timestamp
					AND run_row.deleted_at IS NULL
			) authoritative_run ON TRUE`
		if err := db.WithContext(ctx).Raw(query, args...).Scan(&releaseRows).Error; err != nil {
			return nil, fmt.Errorf("finding authoritative Prow releases for sample payload tags: %w", err)
		}
		for _, row := range releaseRows {
			foundRunIDs.Insert(row.ID)
			if !row.Timestamp.Before(start) && row.Timestamp.Before(end) {
				releases.Insert(row.Release)
			}
		}
	}
	var unresolvedRunIDs []int64
	for _, runID := range runIDs {
		if !foundRunIDs.Has(runID) {
			unresolvedRunIDs = append(unresolvedRunIDs, runID)
		}
	}
	if len(unresolvedRunIDs) > 0 {
		var releaseRows []struct {
			Release string `gorm:"column:prow_job_release"`
		}
		if err := db.WithContext(ctx).Raw(`
			SELECT DISTINCT pjr.prow_job_release
			FROM prow_job_runs pjr
			WHERE pjr.id = ANY(?)
				AND pjr.timestamp >= ? AND pjr.timestamp < ?
				AND pjr.deleted_at IS NULL`,
			pq.Array(unresolvedRunIDs), start, end).Scan(&releaseRows).Error; err != nil {
			return nil, fmt.Errorf("finding Prow releases for runs with missing or stale lookup keys: %w", err)
		}
		for _, row := range releaseRows {
			releases.Insert(row.Release)
		}
	}
	return releases.UnsortedList(), nil
}

// buildSampleRunReleasePartitionFilter constrains the run and test relations to
// releases discovered from the selected sample run filter. testReleaseColumn
// is an internal SQL column reference for the partitioned test relation. A
// non-nil empty slice means the lookup found no matching runs and must return
// no rows.
func buildSampleRunReleasePartitionFilter(releases []string, testReleaseColumn string) (string, []any) {
	if releases == nil {
		return "", nil
	}
	if len(releases) == 0 {
		return "FALSE", nil
	}
	return testReleaseColumn + ".prow_job_run_release = ANY(?) AND pjr.prow_job_release = ANY(?)",
		[]any{pq.Array(releases), pq.Array(releases)}
}
