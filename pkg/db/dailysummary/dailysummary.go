package dailysummary

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

const (
	defaultLookbackDays = 14
	parallelWorkers     = 4
)

var valueColumns = []string{"successes", "failures", "flakes", "runs"}

var (
	insertSQL        = buildInsertSQL()
	onConflictClause = buildOnConflictClause()
)

func buildInsertSQL() string {
	return fmt.Sprintf(`
		INSERT INTO test_daily_summaries (test_id, prow_job_id, suite_id, release, summary_date, %s)
		SELECT
			test_id,
			prow_job_id,
			COALESCE(suite_id, 0),
			prow_job_run_release,
			date(prow_job_run_timestamp),
			COUNT(*) FILTER (WHERE status = 1),
			COUNT(*) FILTER (WHERE status = 12),
			COUNT(*) FILTER (WHERE status = 13),
			COUNT(*)
		FROM prow_job_run_tests
		WHERE prow_job_run_timestamp >= ?::date
		  AND prow_job_run_timestamp < (?::date + INTERVAL '1 day')
		  AND prow_job_run_release = ?
		GROUP BY test_id, prow_job_id, COALESCE(suite_id, 0), prow_job_run_release, date(prow_job_run_timestamp)`,
		strings.Join(valueColumns, ", "))
}

func buildOnConflictClause() string {
	var setClauses, oldCols, newCols []string
	for _, col := range valueColumns {
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
		oldCols = append(oldCols, "test_daily_summaries."+col)
		newCols = append(newCols, "EXCLUDED."+col)
	}

	return fmt.Sprintf(`
		ON CONFLICT (test_id, prow_job_id, suite_id, release, summary_date)
		DO UPDATE SET %s
		WHERE (%s) IS DISTINCT FROM (%s)`,
		strings.Join(setClauses, ", "),
		strings.Join(oldCols, ", "),
		strings.Join(newCols, ", "))
}

type summaryStore interface {
	MaxSummaryDate() (*time.Time, error)
	Truncate() error
	Releases() ([]string, error)
	AggregateRangeForRelease(start, end time.Time, release string, skipConflictDetection bool) error
}

// Options configures the daily summary refresh.
type Options struct {
	Rebuild       bool
	StartOverride *time.Time
	EndOverride   *time.Time
}

// Refresh aggregates prow_job_run_tests into the test_daily_summaries
// table. It runs before matview refreshes so the matviews read from
// pre-aggregated data instead of scanning raw rows.
func Refresh(dbc *db.DB, opts Options) error {
	return refreshSummaries(&pgStore{dbc: dbc}, opts)
}

func refreshSummaries(store summaryStore, opts Options) error {
	loadStart := time.Now()
	log.Info("refreshing daily summaries")

	if opts.Rebuild {
		log.Info("rebuild requested, truncating test_daily_summaries")
		if err := store.Truncate(); err != nil {
			return fmt.Errorf("truncating table: %w", err)
		}
	}

	now := time.Now()

	startDate, endDate, err := dateRange(store, opts, now)
	if err != nil {
		return err
	}

	log.Infof("aggregating daily summaries from %s to %s",
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	releases, err := store.Releases()
	if err != nil {
		return fmt.Errorf("querying releases: %w", err)
	}

	skipConflictDetection := opts.Rebuild
	if !skipConflictDetection {
		maxDate, err := store.MaxSummaryDate()
		if err != nil {
			return fmt.Errorf("checking if table is empty: %w", err)
		}
		skipConflictDetection = maxDate == nil
	}

	if err := aggregateReleases(store, releases, startDate, endDate, skipConflictDetection); err != nil {
		return err
	}

	log.WithField("elapsed", time.Since(loadStart)).Info("daily summary refresh complete")
	return nil
}

func aggregateReleases(store summaryStore, releases []string, startDate, endDate time.Time, skipConflictDetection bool) error {
	errs := make(chan error, len(releases))
	work := make(chan string, len(releases))

	var wg sync.WaitGroup
	for range parallelWorkers {
		wg.Go(func() {
			for release := range work {
				if err := store.AggregateRangeForRelease(startDate, endDate, release, skipConflictDetection); err != nil {
					errs <- fmt.Errorf("aggregating release %s: %w", release, err)
					return
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

// dateRange computes the aggregation window. If explicit overrides were
// provided, those are used directly. Otherwise the start is the last
// summarized date (capped at yesterday) or the default lookback,
// and the end is now.
func dateRange(store summaryStore, opts Options, now time.Time) (time.Time, time.Time, error) {
	if opts.StartOverride != nil && opts.EndOverride != nil {
		return *opts.StartOverride, *opts.EndOverride, nil
	}

	endDate := now
	if opts.EndOverride != nil {
		endDate = *opts.EndOverride
	}

	if opts.StartOverride != nil {
		return *opts.StartOverride, endDate, nil
	}

	startDate, err := resolveStartDate(store, now)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("querying max summary date: %w", err)
	}

	return startDate, endDate, nil
}

// resolveStartDate returns the last summarized date capped at yesterday,
// or the default lookback if no summaries exist.
func resolveStartDate(store summaryStore, now time.Time) (time.Time, error) {
	yesterday := now.AddDate(0, 0, -1)

	maxSummary, err := store.MaxSummaryDate()
	if err != nil {
		return time.Time{}, err
	}
	if maxSummary != nil {
		if maxSummary.Before(yesterday) {
			return *maxSummary, nil
		}
		return yesterday, nil
	}

	return now.AddDate(0, 0, -defaultLookbackDays), nil
}

// pgStore implements summaryStore against PostgreSQL.
type pgStore struct {
	dbc *db.DB
}

func (s *pgStore) MaxSummaryDate() (*time.Time, error) {
	var maxDate *time.Time
	err := s.dbc.DB.Table("test_daily_summaries").
		Select("MAX(summary_date)").Row().Scan(&maxDate)
	return maxDate, err
}

func (s *pgStore) Truncate() error {
	return s.dbc.DB.Exec("TRUNCATE test_daily_summaries").Error
}

func (s *pgStore) Releases() ([]string, error) {
	var releases []string
	err := s.dbc.DB.Table("prow_jobs").Distinct("release").Pluck("release", &releases).Error
	return releases, err
}

func (s *pgStore) AggregateRangeForRelease(startDate, endDate time.Time, release string, skipConflictDetection bool) error {
	sql := insertSQL
	if !skipConflictDetection {
		sql += onConflictClause
	}
	return s.dbc.DB.Exec(sql, startDate, endDate, release).Error
}
