package bugloader

import (
	"context"
	"fmt"
	"strconv"
	"time"

	bq "cloud.google.com/go/bigquery"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db"
)

const ticketRecencyFilter = `t.summary IS NOT NULL
    AND (
      last_changed_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 14 DAY)
      OR (UPPER(t.status.name) NOT IN ('CLOSED', 'VERIFIED')
          AND last_changed_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY))
    )`

const ticketCTE = `WITH TicketData AS (
  SELECT
    t.*,
    c.message AS comment
  FROM
    openshift-ci-data-analysis.jira_data.tickets_dedup t
  LEFT JOIN UNNEST(t.comments) AS c
  WHERE ` + ticketRecencyFilter + `
)
`

const allBugsQuery = `
SELECT
  t.issue.key AS key,
  t.issue.id AS jira_id,
  t.summary AS summary,
  t.last_changed_time AS last_changed_time,
  t.status.name AS status,
  ARRAY(SELECT name FROM UNNEST(affects_versions)) AS affects_versions,
  ARRAY(SELECT name FROM UNNEST(fix_versions)) AS fix_versions,
  ARRAY(SELECT name FROM UNNEST(target_versions)) AS target_versions,
  ARRAY(SELECT name FROM UNNEST(components)) AS components,
  t.labels AS labels,
  t.release_blocker.value AS release_blocker
FROM openshift-ci-data-analysis.jira_data.tickets_dedup t
WHERE ` + ticketRecencyFilter + `
`

const testBugQuery = ticketCTE + `
SELECT DISTINCT
  t.issue.id AS jira_id,
  j.name AS link_name
FROM TicketData t
JOIN openshift-gce-devel.ci_analysis_us.component_mapping_latest j
  ON STRPOS(t.summary, j.name) > 0
  OR STRPOS(t.description, j.name) > 0
  OR STRPOS(t.comment, j.name) > 0
WHERE j.name != "upgrade"
`

const jobBugQuery = ticketCTE + `
SELECT DISTINCT
  t.issue.id AS jira_id,
  j.name AS link_name
FROM TicketData t
JOIN (
  SELECT DISTINCT prowjob_job_name AS name
  FROM openshift-gce-devel.ci_analysis_us.jobs
  WHERE prowjob_job_name IS NOT NULL
    AND prowjob_job_name != ""
) j
ON STRPOS(t.summary, j.name) > 0
OR STRPOS(t.description, j.name) > 0
OR STRPOS(t.comment, j.name) > 0
`

type BugLoader struct {
	ctx    context.Context
	dbc    *db.DB
	bqc    *bigquery.Client
	errors []error
}

type bugRow struct {
	Key             string `bigquery:"key"`
	JiraID          string `bigquery:"jira_id"`
	ID              uint64
	Summary         string           `bigquery:"summary"`
	LastChangedTime bq.NullTimestamp `bigquery:"last_changed_time"`
	Status          string           `bigquery:"status"`
	AffectsVersions []string         `bigquery:"affects_versions"`
	FixVersions     []string         `bigquery:"fix_versions"`
	TargetVersions  []string         `bigquery:"target_versions"`
	Components      []string         `bigquery:"components"`
	Labels          []string         `bigquery:"labels"`
	ReleaseBlocker  string           `bigquery:"release_blocker"`
}

type assocRow struct {
	JiraID   string `bigquery:"jira_id"`
	ID       uint64
	LinkName string `bigquery:"link_name"`
}

func New(ctx context.Context, dbc *db.DB, bqc *bigquery.Client) *BugLoader {
	return &BugLoader{
		ctx: ctx,
		dbc: dbc,
		bqc: bqc,
	}
}

func (bl *BugLoader) Name() string {
	return "bugs"
}

func (bl *BugLoader) Errors() []error {
	return bl.errors
}

func (bl *BugLoader) addError(logger *log.Entry, err error, msg string) {
	logger.WithError(err).Error(msg)
	bl.errors = append(bl.errors, errors.Wrap(err, msg))
}

func (bl *BugLoader) Load() {
	logger := log.WithField("func", "bugloader.Load")

	bugs, err := fetchFromBQ(bl.ctx, bl.bqc, bqlabel.BugLoaderFetchBugs, allBugsQuery, func(b *bugRow) error {
		id, err := strconv.ParseUint(b.JiraID, 10, 64)
		if err != nil {
			bl.addError(logger, err, "skipping bug row: cannot parse jira_id")
			return err
		}
		b.ID = id
		return nil
	})
	if err != nil {
		bl.addError(logger, err, "error fetching bugs from BigQuery")
		return
	}
	logger.WithField("rows", len(bugs)).Info("fetched bugs from BigQuery")

	testAssocs, err := fetchFromBQ(bl.ctx, bl.bqc, bqlabel.BugLoaderTestBugs, testBugQuery, bl.assocTransform(logger, "test-bug"))
	if err != nil {
		bl.addError(logger, err, "error fetching test-bug associations from BigQuery")
		return
	}
	logger.WithField("rows", len(testAssocs)).Info("fetched test-bug associations from BigQuery")

	jobAssocs, err := fetchFromBQ(bl.ctx, bl.bqc, bqlabel.BugLoaderJobBugs, jobBugQuery, bl.assocTransform(logger, "job-bug"))
	if err != nil {
		bl.addError(logger, err, "error fetching job-bug associations from BigQuery")
		return
	}
	logger.WithField("rows", len(jobAssocs)).Info("fetched job-bug associations from BigQuery")

	if err := bl.loadIntoDB(bugs, testAssocs, jobAssocs); err != nil {
		bl.addError(logger, err, "error loading bugs into database")
	}
}

func fetchFromBQ[T any](ctx context.Context, bqc *bigquery.Client, label bqlabel.QueryValue, query string, transform func(*T) error) ([]T, error) {
	q := bqc.Query(ctx, label, query)
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute query")
	}

	var rows []T
	for {
		var row T
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.WithMessage(err, "failed to iterate over results")
		}
		if transform != nil {
			if err := transform(&row); err != nil {
				continue
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (bl *BugLoader) assocTransform(logger *log.Entry, kind string) func(*assocRow) error {
	return func(a *assocRow) error {
		if a.LinkName == "" {
			err := fmt.Errorf("empty link name")
			bl.addError(logger, err, fmt.Sprintf("skipping %s association row", kind))
			return err
		}
		id, err := strconv.ParseUint(a.JiraID, 10, 64)
		if err != nil {
			bl.addError(logger, err, fmt.Sprintf("skipping %s association row: cannot parse jira_id", kind))
			return err
		}
		a.ID = id
		return nil
	}
}

func bugTempColumns() []db.TempColumn[bugRow] {
	return []db.TempColumn[bugRow]{
		{Name: "id", Type: "bigint NOT NULL", Value: func(b *bugRow) any { return b.ID }},
		{Name: "key", Type: "text NOT NULL", Value: func(b *bugRow) any { return b.Key }},
		{Name: "status", Type: "text NOT NULL", Value: func(b *bugRow) any { return b.Status }},
		{Name: "last_change_time", Type: "timestamptz NOT NULL", Value: func(b *bugRow) any {
			if b.LastChangedTime.Valid {
				return b.LastChangedTime.Timestamp
			}
			return time.Now()
		}},
		{Name: "summary", Type: "text NOT NULL", Value: func(b *bugRow) any { return b.Summary }},
		{Name: "affects_versions", Type: "text[]", Value: func(b *bugRow) any { return pq.StringArray(b.AffectsVersions) }},
		{Name: "fix_versions", Type: "text[]", Value: func(b *bugRow) any { return pq.StringArray(b.FixVersions) }},
		{Name: "target_versions", Type: "text[]", Value: func(b *bugRow) any { return pq.StringArray(b.TargetVersions) }},
		{Name: "components", Type: "text[]", Value: func(b *bugRow) any { return pq.StringArray(b.Components) }},
		{Name: "labels", Type: "text[]", Value: func(b *bugRow) any { return pq.StringArray(b.Labels) }},
		{Name: "url", Type: "text NOT NULL", Value: func(b *bugRow) any {
			return fmt.Sprintf("https://redhat.atlassian.net/browse/%s", b.Key)
		}},
		{Name: "release_blocker", Type: "text NOT NULL", Value: func(b *bugRow) any { return b.ReleaseBlocker }},
	}
}

func assocTempColumns() []db.TempColumn[assocRow] {
	return []db.TempColumn[assocRow]{
		{Name: "id", Type: "bigint NOT NULL", Value: func(a *assocRow) any { return a.ID }},
		{Name: "link_name", Type: "text NOT NULL", Value: func(a *assocRow) any { return a.LinkName }},
	}
}

func (bl *BugLoader) loadIntoDB(bugs []bugRow, testAssocs, jobAssocs []assocRow) error {
	sqlDB, err := bl.dbc.DB.DB()
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

	tx, err := conn.Begin(bl.ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(bl.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			log.WithError(err).Error("failed to rollback transaction")
		}
	}()

	cleanupBugs, err := db.CopyToTempTable(bl.ctx, tx, "tmp_bugs", bugs, bugTempColumns())
	defer cleanupBugs()
	if err != nil {
		return err
	}

	cleanupTestAssocs, err := db.CopyToTempTable(bl.ctx, tx, "tmp_test_assocs", testAssocs, assocTempColumns())
	defer cleanupTestAssocs()
	if err != nil {
		return err
	}

	cleanupJobAssocs, err := db.CopyToTempTable(bl.ctx, tx, "tmp_job_assocs", jobAssocs, assocTempColumns())
	defer cleanupJobAssocs()
	if err != nil {
		return err
	}

	if err := bl.upsertBugs(tx); err != nil {
		return err
	}
	if err := bl.syncTestAssociations(tx); err != nil {
		return err
	}
	if err := bl.syncJobAssociations(tx); err != nil {
		return err
	}
	if err := bl.reconcileTriages(tx); err != nil {
		return err
	}

	if err := tx.Commit(bl.ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (bl *BugLoader) upsertBugs(conn db.PgxSession) error {
	st := time.Now()
	tag, err := conn.Exec(bl.ctx, `
		INSERT INTO bugs (
			id, key, status, last_change_time, summary,
			affects_versions, fix_versions, target_versions,
			components, labels, url, release_blocker,
			created_at, updated_at
		)
		SELECT
			id, key, status, last_change_time, summary,
			affects_versions, fix_versions, target_versions,
			components, labels, url, release_blocker,
			NOW(), NOW()
		FROM tmp_bugs b
		WHERE EXISTS (SELECT 1 FROM tmp_test_assocs ta WHERE ta.id = b.id)
		   OR EXISTS (SELECT 1 FROM tmp_job_assocs ja WHERE ja.id = b.id)
		   OR EXISTS (SELECT 1 FROM triages t WHERE t.url = b.url AND t.url != '')
		ON CONFLICT (id) DO UPDATE SET
			key              = EXCLUDED.key,
			status           = EXCLUDED.status,
			last_change_time = EXCLUDED.last_change_time,
			summary          = EXCLUDED.summary,
			affects_versions = EXCLUDED.affects_versions,
			fix_versions     = EXCLUDED.fix_versions,
			target_versions  = EXCLUDED.target_versions,
			components       = EXCLUDED.components,
			labels           = EXCLUDED.labels,
			url              = EXCLUDED.url,
			release_blocker  = EXCLUDED.release_blocker,
			updated_at       = NOW()
		WHERE bugs.key              IS DISTINCT FROM EXCLUDED.key
		   OR bugs.status           IS DISTINCT FROM EXCLUDED.status
		   OR bugs.last_change_time IS DISTINCT FROM EXCLUDED.last_change_time
		   OR bugs.summary          IS DISTINCT FROM EXCLUDED.summary
		   OR bugs.affects_versions IS DISTINCT FROM EXCLUDED.affects_versions
		   OR bugs.fix_versions     IS DISTINCT FROM EXCLUDED.fix_versions
		   OR bugs.target_versions  IS DISTINCT FROM EXCLUDED.target_versions
		   OR bugs.components       IS DISTINCT FROM EXCLUDED.components
		   OR bugs.labels           IS DISTINCT FROM EXCLUDED.labels
		   OR bugs.url              IS DISTINCT FROM EXCLUDED.url
		   OR bugs.release_blocker  IS DISTINCT FROM EXCLUDED.release_blocker
	`)
	if err != nil {
		return fmt.Errorf("upserting bugs: %w", err)
	}
	log.WithFields(log.Fields{"rows": tag.RowsAffected(), "elapsed": time.Since(st)}).Info("upsert bugs complete")
	return nil
}

const desiredBugTests = `
	SELECT DISTINCT bug_id, test_id FROM (
		SELECT ta.id AS bug_id, t.id AS test_id
		FROM tmp_test_assocs ta
		INNER JOIN bugs b ON b.id = ta.id AND b.deleted_at IS NULL
		INNER JOIN tests t ON t.name = ta.link_name AND t.deleted_at IS NULL
		UNION ALL
		SELECT ta.id, t2.id
		FROM tmp_test_assocs ta
		INNER JOIN bugs b ON b.id = ta.id AND b.deleted_at IS NULL
		INNER JOIN test_ownerships to1 ON to1.name = ta.link_name
		INNER JOIN test_ownerships to2 ON to2.unique_id = to1.unique_id AND to2.unique_id != ''
		INNER JOIN tests t2 ON t2.name = to2.name AND t2.deleted_at IS NULL
	) matches
`

func (bl *BugLoader) syncTestAssociations(conn db.PgxSession) error {
	st := time.Now()

	deleteTag, err := conn.Exec(bl.ctx, `
		DELETE FROM bug_tests bt
		WHERE bt.bug_id IN (SELECT id FROM tmp_bugs)
		  AND NOT EXISTS (
			SELECT 1 FROM (`+desiredBugTests+`) d
			WHERE d.bug_id = bt.bug_id AND d.test_id = bt.test_id
		  )
	`)
	if err != nil {
		return fmt.Errorf("deleting stale bug_tests: %w", err)
	}

	insertTag, err := conn.Exec(bl.ctx, `
		INSERT INTO bug_tests (bug_id, test_id)`+desiredBugTests+`
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("inserting bug_tests: %w", err)
	}

	log.WithFields(log.Fields{
		"deleted":  deleteTag.RowsAffected(),
		"inserted": insertTag.RowsAffected(),
		"elapsed":  time.Since(st),
	}).Info("sync bug_tests complete")
	return nil
}

const desiredBugJobs = `
	SELECT DISTINCT ja.id AS bug_id, j.id AS prow_job_id
	FROM tmp_job_assocs ja
	INNER JOIN bugs b ON b.id = ja.id AND b.deleted_at IS NULL
	INNER JOIN prow_jobs j ON j.name = ja.link_name AND j.deleted_at IS NULL
`

func (bl *BugLoader) syncJobAssociations(conn db.PgxSession) error {
	st := time.Now()

	deleteTag, err := conn.Exec(bl.ctx, `
		DELETE FROM bug_jobs bj
		WHERE bj.bug_id IN (SELECT id FROM tmp_bugs)
		  AND NOT EXISTS (
			SELECT 1 FROM (`+desiredBugJobs+`) d
			WHERE d.bug_id = bj.bug_id AND d.prow_job_id = bj.prow_job_id
		  )
	`)
	if err != nil {
		return fmt.Errorf("deleting stale bug_jobs: %w", err)
	}

	insertTag, err := conn.Exec(bl.ctx, `
		INSERT INTO bug_jobs (bug_id, prow_job_id)`+desiredBugJobs+`
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("inserting bug_jobs: %w", err)
	}

	log.WithFields(log.Fields{
		"deleted":  deleteTag.RowsAffected(),
		"inserted": insertTag.RowsAffected(),
		"elapsed":  time.Since(st),
	}).Info("sync bug_jobs complete")
	return nil
}

func (bl *BugLoader) reconcileTriages(conn db.PgxSession) error {
	st := time.Now()

	// Update triage descriptions from their linked bug's summary
	descTag, err := conn.Exec(bl.ctx, `
		UPDATE triages t
		SET description = b.summary, updated_at = NOW()
		FROM bugs b
		WHERE b.url = t.url
		  AND b.deleted_at IS NULL
		  AND t.url != ''
		  AND b.summary != ''
		  AND b.summary != t.description
	`)
	if err != nil {
		return fmt.Errorf("updating triage descriptions: %w", err)
	}

	// Link triages to their bug records
	linkTag, err := conn.Exec(bl.ctx, `
		UPDATE triages t
		SET bug_id = b.id, updated_at = NOW()
		FROM bugs b
		WHERE b.url = t.url
		  AND b.deleted_at IS NULL
		  AND t.url != ''
		  AND (t.bug_id IS NULL OR t.bug_id != b.id)
	`)
	if err != nil {
		return fmt.Errorf("linking triages to bugs: %w", err)
	}

	// Auto-resolve triages where the bug has progressed and the triage
	// covers only a single release
	resolveTag, err := conn.Exec(bl.ctx, `
		UPDATE triages t
		SET resolved = NOW(), resolution_reason = 'jira-progression', updated_at = NOW()
		FROM bugs b
		WHERE b.url = t.url
		  AND b.deleted_at IS NULL
		  AND t.url != ''
		  AND t.resolved IS NULL
		  AND b.status IN ('ON_QA', 'Verified', 'Release Pending', 'Closed')
		  AND (
			SELECT COUNT(DISTINCT r.release)
			FROM triage_regressions tr
			INNER JOIN test_regressions r ON r.id = tr.test_regression_id
			WHERE tr.triage_id = t.id
		  ) = 1
	`)
	if err != nil {
		return fmt.Errorf("auto-resolving triages: %w", err)
	}

	log.WithFields(log.Fields{
		"descriptions_updated": descTag.RowsAffected(),
		"bugs_linked":          linkTag.RowsAffected(),
		"auto_resolved":        resolveTag.RowsAffected(),
		"elapsed":              time.Since(st),
	}).Info("triage reconciliation complete")
	return nil
}
