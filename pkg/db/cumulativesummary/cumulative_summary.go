package cumulativesummary

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

type summaryStore interface {
	MaxCumulativeSummaryDate() (*civil.Date, error)
	MaxDailySummaryDate() (*civil.Date, error)
	Releases() ([]string, error)
	UpdateDateForRelease(date civil.Date, release string) error
}

// Refresh computes cumulative summaries from test_daily_totals.
// earliestChanged is the earliest date that test_daily_totals was
// updated; cumulative summaries from that date forward will be recomputed.
// Returns the earliest date that was updated so downstream consumers
// (variant cumulative summaries) know where to start.
func Refresh(dbc *db.DB, earliestChanged civil.Date) (civil.Date, error) {
	return doRefresh(&pgStore{dbc: dbc}, earliestChanged)
}

// Backfill processes an explicit date range without automatic date detection.
func Backfill(dbc *db.DB, startDate, endDate civil.Date) error {
	store := &pgStore{dbc: dbc}
	_, err := refreshDateRange(store, startDate, endDate)
	return err
}

func doRefresh(store summaryStore, earliestChanged civil.Date) (civil.Date, error) {
	return doRefreshWithToday(store, earliestChanged, civil.DateOf(time.Now().UTC()))
}

func doRefreshWithToday(store summaryStore, earliestChanged, today civil.Date) (civil.Date, error) {
	log.Info("refreshing cumulative summaries")

	maxCumulativeSummaryDate, err := store.MaxCumulativeSummaryDate()
	if err != nil {
		return civil.Date{}, fmt.Errorf("checking max cumulative summary date: %w", err)
	}

	maxDailySummaryDate, err := store.MaxDailySummaryDate()
	if err != nil {
		return civil.Date{}, fmt.Errorf("checking max daily summary date: %w", err)
	}
	if maxDailySummaryDate == nil {
		log.Info("no daily summaries found, nothing to update")
		return earliestChanged, nil
	}
	endDate := *maxDailySummaryDate

	startDate := resolveStartDate(earliestChanged, maxCumulativeSummaryDate, today)

	return refreshDateRange(store, startDate, endDate)
}

func refreshDateRange(store summaryStore, startDate, endDate civil.Date) (civil.Date, error) {
	if startDate.After(endDate) {
		return startDate, nil
	}
	loadStart := time.Now()
	days := endDate.DaysSince(startDate) + 1

	releases, err := store.Releases()
	if err != nil {
		return civil.Date{}, fmt.Errorf("querying releases: %w", err)
	}

	log.WithFields(log.Fields{
		"start":    startDate,
		"end":      endDate,
		"days":     days,
		"releases": len(releases),
	}).Info("updating cumulative summaries")

	for date := startDate; !date.After(endDate); date = date.AddDays(1) {
		updateStart := time.Now()
		for _, release := range releases {
			if err := store.UpdateDateForRelease(date, release); err != nil {
				return civil.Date{}, fmt.Errorf("updating cumulative summaries for %s release %s: %w", date, release, err)
			}
		}
		log.WithFields(log.Fields{
			"date":    date,
			"elapsed": time.Since(updateStart),
		}).Info("updated cumulative summaries for date")
	}

	log.WithField("elapsed", time.Since(loadStart)).Info("cumulative summary refresh complete")
	return startDate, nil
}

type pgStore struct {
	dbc *db.DB
}

func (s *pgStore) MaxCumulativeSummaryDate() (*civil.Date, error) {
	var d *civil.Date
	err := s.dbc.DB.Table("test_cumulative_summaries").
		Select("MAX(date)").Row().Scan(&d)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (s *pgStore) MaxDailySummaryDate() (*civil.Date, error) {
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

func (s *pgStore) UpdateDateForRelease(date civil.Date, release string) error {
	return s.dbc.DB.Exec(`
		INSERT INTO test_cumulative_summaries (date, test_id, prow_job_id, suite_id, release,
		                         prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs)
		SELECT
			?,
			COALESCE(prev.test_id, tds.test_id),
			COALESCE(prev.prow_job_id, tds.prow_job_id),
			COALESCE(prev.suite_id, tds.suite_id),
			COALESCE(prev.release, tds.release),
			COALESCE(prev.prefix_sum_successes, 0) + COALESCE(tds.successes, 0),
			COALESCE(prev.prefix_sum_failures, 0) + COALESCE(tds.failures, 0),
			COALESCE(prev.prefix_sum_flakes, 0) + COALESCE(tds.flakes, 0),
			COALESCE(prev.prefix_sum_runs, 0) + COALESCE(tds.runs, 0)
		FROM (SELECT * FROM test_cumulative_summaries WHERE date = ?::date - 1 AND release = ?) prev
		FULL OUTER JOIN (SELECT * FROM test_daily_totals WHERE date = ? AND release = ?) tds
			ON prev.test_id = tds.test_id
			AND prev.prow_job_id = tds.prow_job_id
			AND prev.suite_id = tds.suite_id
		ON CONFLICT (date, release, test_id, prow_job_id, suite_id)
		DO UPDATE SET
			prefix_sum_successes = EXCLUDED.prefix_sum_successes,
			prefix_sum_failures = EXCLUDED.prefix_sum_failures,
			prefix_sum_flakes = EXCLUDED.prefix_sum_flakes,
			prefix_sum_runs = EXCLUDED.prefix_sum_runs
		WHERE (test_cumulative_summaries.prefix_sum_successes, test_cumulative_summaries.prefix_sum_failures,
		       test_cumulative_summaries.prefix_sum_flakes, test_cumulative_summaries.prefix_sum_runs)
		   IS DISTINCT FROM
		      (EXCLUDED.prefix_sum_successes, EXCLUDED.prefix_sum_failures,
		       EXCLUDED.prefix_sum_flakes, EXCLUDED.prefix_sum_runs)
	`, date, date, release, date, release).Error
}
