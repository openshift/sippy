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

const parallelWorkers = 4

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
	JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id AND pjr.prow_job_release = pjrt.prow_job_run_release AND pjr.timestamp = pjrt.prow_job_run_timestamp
	WHERE pjrt.prow_job_run_timestamp >= ?::date
	  AND pjrt.prow_job_run_timestamp < (?::date + INTERVAL '1 day')
	  AND pjrt.prow_job_run_release = ?
	  AND (pjr.labels IS NULL OR NOT (pjr.labels @> ARRAY['InfraFailure']))
	GROUP BY pjrt.test_id, pjrt.prow_job_id, COALESCE(pjrt.suite_id, 0), pjrt.lifecycle, pjrt.prow_job_run_release, date(pjrt.prow_job_run_timestamp)`

type summaryStore interface {
	Releases() ([]string, error)
	ReplaceRangeForRelease(start, end civil.Date, release string) error
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

type pgStore struct {
	dbc *db.DB
}

func (s *pgStore) Releases() ([]string, error) {
	var releases []string
	err := s.dbc.DB.Table("release_definitions").
		Order("major DESC, minor DESC").
		Pluck("release", &releases).Error
	return releases, err
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
