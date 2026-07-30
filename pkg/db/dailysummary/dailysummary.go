package dailysummary

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
)

const (
	defaultLookbackDays = 14
	parallelWorkers     = 4
)

var valueColumns = []string{
	"successes", "failures", "flakes", "runs",
	"first_failure_timestamp", "last_failure_timestamp",
	"first_success_timestamp", "last_success_timestamp",
}

func buildInsertSQL(tableName, dateColumn string) string {
	return fmt.Sprintf(`
		INSERT INTO %s (test_id, prow_job_id, suite_id, lifecycle, release, %s, %s)
		SELECT
			pjrt.test_id,
			pjrt.prow_job_id,
			COALESCE(pjrt.suite_id, 0),
			pjrt.lifecycle,
			pjrt.prow_job_run_release,
			date(pjrt.prow_job_run_timestamp),
			COUNT(*) FILTER (WHERE pjrt.status = 1),
			COUNT(*) FILTER (WHERE pjrt.status = 12),
			COUNT(*) FILTER (WHERE pjrt.status = 13),
			COUNT(*),
			MIN(pjrt.prow_job_run_timestamp) FILTER (WHERE pjrt.status = 12),
			MAX(pjrt.prow_job_run_timestamp) FILTER (WHERE pjrt.status = 12),
			MIN(pjrt.prow_job_run_timestamp) FILTER (WHERE pjrt.status = 1),
			MAX(pjrt.prow_job_run_timestamp) FILTER (WHERE pjrt.status = 1)
		FROM prow_job_run_tests pjrt
		WHERE pjrt.prow_job_run_timestamp >= ?::date
		  AND pjrt.prow_job_run_timestamp < (?::date + INTERVAL '1 day')
		  AND pjrt.prow_job_run_release = ?
		GROUP BY pjrt.test_id, pjrt.prow_job_id, COALESCE(pjrt.suite_id, 0), pjrt.lifecycle, pjrt.prow_job_run_release, date(pjrt.prow_job_run_timestamp)`,
		tableName, dateColumn, strings.Join(valueColumns, ", "))
}

func buildOnConflictClause(tableName, dateColumn string) string {
	var setClauses, oldCols, newCols []string
	for _, col := range valueColumns {
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
		oldCols = append(oldCols, tableName+"."+col)
		newCols = append(newCols, "EXCLUDED."+col)
	}

	return fmt.Sprintf(`
		ON CONFLICT (release, %s, test_id, suite_id, lifecycle, prow_job_id)
		DO UPDATE SET %s
		WHERE (%s) IS DISTINCT FROM (%s)`,
		dateColumn,
		strings.Join(setClauses, ", "),
		strings.Join(oldCols, ", "),
		strings.Join(newCols, ", "))
}

// buildCleanupDeleteSQL removes rows whose key no longer has any
// supporting raw result. ON CONFLICT DO UPDATE can only ever touch keys
// present in the freshly aggregated data - it can never notice that a
// previously-valid key (e.g. a test's lifecycle was reclassified, or its
// suite changed, or the underlying job run was reprocessed away) no
// longer applies. This is a separate, targeted delete rather than
// DELETE-then-INSERT so it only touches genuinely defunct rows instead of
// unconditionally churning every row in the range on every run.
func buildCleanupDeleteSQL(tableName, dateColumn string) string {
	return fmt.Sprintf(`
		DELETE FROM %[1]s dt
		WHERE dt.release = ?
		  AND dt.%[2]s >= ?
		  AND dt.%[2]s <= ?
		  AND NOT EXISTS (
		      SELECT 1 FROM prow_job_run_tests pjrt
		      WHERE pjrt.prow_job_run_release = dt.release
		        AND pjrt.test_id = dt.test_id
		        AND pjrt.prow_job_id = dt.prow_job_id
		        AND COALESCE(pjrt.suite_id, 0) = dt.suite_id
		        AND pjrt.lifecycle = dt.lifecycle
		        AND date(pjrt.prow_job_run_timestamp) = dt.%[2]s
		  )`,
		tableName, dateColumn)
}

type summaryStore interface {
	MaxSummaryDate() (*civil.Date, error)
	Releases() ([]string, error)
	AggregateRangeForRelease(start, end civil.Date, release string, skipConflictDetection bool) error
}

func refreshSummaries(store summaryStore) (civil.Date, error) {
	loadStart := time.Now()
	log.Info("refreshing daily summaries")

	today := civil.DateOf(time.Now().UTC())
	maxDate, err := store.MaxSummaryDate()
	if err != nil {
		return civil.Date{}, fmt.Errorf("querying max summary date: %w", err)
	}

	startDate := startDateFromMax(maxDate, today)
	skipConflictDetection := maxDate == nil

	if err := doAggregate(store, startDate, today, skipConflictDetection, loadStart); err != nil {
		return civil.Date{}, err
	}
	return startDate, nil
}

// Refresh aggregates prow_job_run_tests into the partitioned
// test_daily_totals table. Returns the earliest date that was refreshed
// so downstream consumers (cumulative summaries) know which dates
// may have changed.
func Refresh(dbc *db.DB) (civil.Date, error) {
	return refreshSummaries(&pgStore{dbc: dbc, tableName: "test_daily_totals", dateColumn: "date"})
}

// Backfill processes an explicit date range without automatic date detection.
func Backfill(dbc *db.DB, startDate, endDate civil.Date) error {
	return backfillSummaries(&pgStore{dbc: dbc, tableName: "test_daily_totals", dateColumn: "date"}, startDate, endDate)
}

func backfillSummaries(store summaryStore, startDate, endDate civil.Date) error {
	loadStart := time.Now()
	releases, err := store.Releases()
	if err != nil {
		return fmt.Errorf("querying releases: %w", err)
	}

	days := endDate.DaysSince(startDate) + 1
	log.WithFields(log.Fields{
		"start":    startDate,
		"end":      endDate,
		"days":     days,
		"releases": len(releases),
	}).Info("backfilling daily summaries")

	for date := startDate; !date.After(endDate); date = date.AddDays(1) {
		dayStart := time.Now()
		if err := aggregateReleases(store, releases, date, date, false); err != nil {
			return fmt.Errorf("backfilling %s: %w", date, err)
		}
		log.WithFields(log.Fields{
			"date":    date,
			"elapsed": time.Since(dayStart),
		}).Info("backfilled daily summaries for date")
	}

	log.WithField("elapsed", time.Since(loadStart)).Info("daily summary backfill complete")
	return nil
}

func doAggregate(store summaryStore, startDate, endDate civil.Date, skipConflictDetection bool, loadStart time.Time) error {
	releases, err := store.Releases()
	if err != nil {
		return fmt.Errorf("querying releases: %w", err)
	}

	days := endDate.DaysSince(startDate) + 1
	log.WithFields(log.Fields{
		"start":    startDate,
		"end":      endDate,
		"days":     days,
		"releases": len(releases),
	}).Info("aggregating daily summaries")

	for date := startDate; !date.After(endDate); date = date.AddDays(1) {
		dayStart := time.Now()
		if err := aggregateReleases(store, releases, date, date, skipConflictDetection); err != nil {
			return fmt.Errorf("aggregating %s: %w", date, err)
		}
		log.WithFields(log.Fields{
			"date":    date,
			"elapsed": time.Since(dayStart),
		}).Debug("aggregated daily summaries for date")
	}

	log.WithField("elapsed", time.Since(loadStart)).Info("daily summary refresh complete")
	return nil
}

func aggregateReleases(store summaryStore, releases []string, startDate, endDate civil.Date, skipConflictDetection bool) error {
	errs := make(chan error, len(releases))
	work := make(chan string, len(releases))

	var wg sync.WaitGroup
	for range parallelWorkers {
		wg.Go(func() {
			for release := range work {
				if err := store.AggregateRangeForRelease(startDate, endDate, release, skipConflictDetection); err != nil {
					errs <- fmt.Errorf("aggregating release %s: %w", release, err)
					continue
				}
				log.WithField("release", release).Debug("aggregated daily summary for release")
			}
		})
	}

	for _, release := range releases {
		work <- release
	}
	close(work)
	wg.Wait()
	close(errs)

	var combined []error
	for err := range errs {
		combined = append(combined, err)
	}
	return errors.Join(combined...)
}

func startDateFromMax(maxSummary *civil.Date, today civil.Date) civil.Date {
	yesterday := today.AddDays(-1)
	if maxSummary != nil {
		if maxSummary.Before(yesterday) {
			return *maxSummary
		}
		return yesterday
	}
	return today.AddDays(-defaultLookbackDays)
}

type pgStore struct {
	dbc        *db.DB
	tableName  string
	dateColumn string
}

func (s *pgStore) MaxSummaryDate() (*civil.Date, error) {
	var d *civil.Date
	err := s.dbc.DB.Table(s.tableName).
		Select(fmt.Sprintf("MAX(%s)", s.dateColumn)).Row().Scan(&d)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (s *pgStore) Releases() ([]string, error) {
	var releases []string
	err := s.dbc.DB.Table("release_definitions").
		Order("major DESC, minor DESC").
		Pluck("release", &releases).Error
	return releases, err
}

func (s *pgStore) AggregateRangeForRelease(startDate, endDate civil.Date, release string, skipConflictDetection bool) error {
	insertSQL := buildInsertSQL(s.tableName, s.dateColumn)
	if skipConflictDetection {
		// Table is known empty for this range, so there is nothing to
		// merge with or clean up - skip both the conflict clause and the
		// cleanup delete.
		return s.dbc.DB.Exec(insertSQL, startDate, endDate, release).Error
	}
	insertSQL += buildOnConflictClause(s.tableName, s.dateColumn)
	cleanupSQL := buildCleanupDeleteSQL(s.tableName, s.dateColumn)
	return s.dbc.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(insertSQL, startDate, endDate, release).Error; err != nil {
			return err
		}
		return tx.Exec(cleanupSQL, release, startDate, endDate).Error
	})
}
