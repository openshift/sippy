package dailysummary

import (
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
)

const (
	defaultLookbackDays = 14
	parallelWorkers     = 4
)

const insertSQL = `
	INSERT INTO test_daily_totals (test_id, prow_job_id, suite_id, lifecycle, release, date,
		successes, failures, flakes, runs,
		first_failure_timestamp, last_failure_timestamp,
		first_success_timestamp, last_success_timestamp)
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
	GROUP BY pjrt.test_id, pjrt.prow_job_id, COALESCE(pjrt.suite_id, 0), pjrt.lifecycle, pjrt.prow_job_run_release, date(pjrt.prow_job_run_timestamp)`

const onConflictClause = `
		ON CONFLICT (release, date, test_id, suite_id, lifecycle, prow_job_id)
		DO UPDATE SET
			successes = EXCLUDED.successes,
			failures = EXCLUDED.failures,
			flakes = EXCLUDED.flakes,
			runs = EXCLUDED.runs,
			first_failure_timestamp = EXCLUDED.first_failure_timestamp,
			last_failure_timestamp = EXCLUDED.last_failure_timestamp,
			first_success_timestamp = EXCLUDED.first_success_timestamp,
			last_success_timestamp = EXCLUDED.last_success_timestamp
		WHERE (test_daily_totals.successes, test_daily_totals.failures,
		       test_daily_totals.flakes, test_daily_totals.runs,
		       test_daily_totals.first_failure_timestamp, test_daily_totals.last_failure_timestamp,
		       test_daily_totals.first_success_timestamp, test_daily_totals.last_success_timestamp)
		   IS DISTINCT FROM
		      (EXCLUDED.successes, EXCLUDED.failures,
		       EXCLUDED.flakes, EXCLUDED.runs,
		       EXCLUDED.first_failure_timestamp, EXCLUDED.last_failure_timestamp,
		       EXCLUDED.first_success_timestamp, EXCLUDED.last_success_timestamp)`

type summaryStore interface {
	MaxSummaryDate() (*civil.Date, error)
	Releases() ([]string, error)
	AggregateRangeForRelease(start, end civil.Date, release string, skipConflictDetection bool) error
	ReplaceRangeForRelease(start, end civil.Date, release string) error
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
	return refreshSummaries(&pgStore{dbc: dbc})
}

// Backfill processes an explicit date range without automatic date detection.
func Backfill(dbc *db.DB, startDate, endDate civil.Date) error {
	return backfillSummaries(&pgStore{dbc: dbc}, startDate, endDate)
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
		if err := replaceReleases(store, releases, date); err != nil {
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

func replaceReleases(store summaryStore, releases []string, date civil.Date) error {
	g := new(errgroup.Group)
	g.SetLimit(parallelWorkers)
	for _, release := range releases {
		g.Go(func() error {
			if err := store.ReplaceRangeForRelease(date, date, release); err != nil {
				return fmt.Errorf("replacing release %s: %w", release, err)
			}
			return nil
		})
	}
	return g.Wait()
}

func aggregateReleases(store summaryStore, releases []string, startDate, endDate civil.Date, skipConflictDetection bool) error {
	g := new(errgroup.Group)
	g.SetLimit(parallelWorkers)
	for _, release := range releases {
		g.Go(func() error {
			if err := store.AggregateRangeForRelease(startDate, endDate, release, skipConflictDetection); err != nil {
				return fmt.Errorf("aggregating release %s: %w", release, err)
			}
			log.WithField("release", release).Debug("aggregated daily summary for release")
			return nil
		})
	}
	return g.Wait()
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
	dbc *db.DB
}

func (s *pgStore) MaxSummaryDate() (*civil.Date, error) {
	var d *civil.Date
	err := s.dbc.DB.Table("test_daily_totals").
		Select("MAX(date)").Row().Scan(&d)
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
	if skipConflictDetection {
		return s.dbc.DB.Exec(insertSQL, startDate, endDate, release).Error
	}
	return s.dbc.DB.Exec(insertSQL+onConflictClause, startDate, endDate, release).Error
}

const replaceDeleteSQL = `DELETE FROM test_daily_totals WHERE release = ? AND date >= ? AND date <= ?`

func (s *pgStore) ReplaceRangeForRelease(startDate, endDate civil.Date, release string) error {
	return s.dbc.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(replaceDeleteSQL, release, startDate, endDate).Error; err != nil {
			return err
		}
		return tx.Exec(insertSQL, startDate, endDate, release).Error
	})
}
