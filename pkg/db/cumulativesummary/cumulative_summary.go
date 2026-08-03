package cumulativesummary

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

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

// Delete+insert (not upsert) so that rows whose daily-total key changed
// (e.g. lifecycle reclassification) are dropped instead of persisting forever.
const deleteSQL = `DELETE FROM test_cumulative_summaries WHERE release = ? AND date = ?`

const insertSQL = `
	INSERT INTO test_cumulative_summaries (date, test_id, prow_job_id, suite_id, lifecycle, release,
	                         prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs,
	                         prefix_max_last_failure, prefix_max_last_success)
	SELECT
		?,
		COALESCE(prev.test_id, tds.test_id),
		COALESCE(prev.prow_job_id, tds.prow_job_id),
		COALESCE(prev.suite_id, tds.suite_id),
		COALESCE(prev.lifecycle, tds.lifecycle),
		COALESCE(prev.release, tds.release),
		COALESCE(prev.prefix_sum_successes, 0) + COALESCE(tds.successes, 0),
		COALESCE(prev.prefix_sum_failures, 0) + COALESCE(tds.failures, 0),
		COALESCE(prev.prefix_sum_flakes, 0) + COALESCE(tds.flakes, 0),
		COALESCE(prev.prefix_sum_runs, 0) + COALESCE(tds.runs, 0),
		GREATEST(prev.prefix_max_last_failure, tds.last_failure_timestamp),
		GREATEST(prev.prefix_max_last_success, tds.last_success_timestamp)
	FROM (SELECT * FROM test_cumulative_summaries WHERE date = ?::date - 1 AND release = ?) prev
	FULL OUTER JOIN (SELECT * FROM test_daily_totals WHERE date = ? AND release = ?) tds
		ON prev.test_id = tds.test_id
		AND prev.prow_job_id = tds.prow_job_id
		AND prev.suite_id = tds.suite_id
		AND prev.lifecycle = tds.lifecycle`

func (s *pgStore) UpdateDateForRelease(date civil.Date, release string) error {
	return s.dbc.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(deleteSQL, release, date).Error; err != nil {
			return err
		}
		return tx.Exec(insertSQL, date, date, release, date, release).Error
	})
}
