package gateststatus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// GATestStatusLoader snapshots eligible PostgreSQL Prow test results at GA.
type GATestStatusLoader struct {
	ctx      context.Context
	dbc      *db.DB
	force    bool
	releases []string
	errs     []error
}

// New constructs the GA snapshot loader. Force permits an explicit rebuild of
// restored historical partitions as well as refreshing current snapshots.
func New(ctx context.Context, dbc *db.DB, force bool, releases []string) *GATestStatusLoader {
	return &GATestStatusLoader{ctx: ctx, dbc: dbc, force: force, releases: releases}
}

func (l *GATestStatusLoader) Name() string    { return "ga-test-status" }
func (l *GATestStatusLoader) Errors() []error { return l.errs }

// gaSnapshotCutoff returns the latest GA date eligible at the given time.
func gaSnapshotCutoff(now time.Time) civil.Date {
	return civil.DateOf(now.UTC()).AddDays(-3)
}

func (l *GATestStatusLoader) Load() {
	var releases []models.ReleaseDefinition
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -db.DefaultPartitionRetentionDays).Truncate(24 * time.Hour)
	q := l.dbc.DB.WithContext(l.ctx).Where("ga_date <= ?", gaSnapshotCutoff(now))
	if len(l.releases) != 0 {
		q = q.Where("release IN ?", l.releases)
	}
	if err := q.Find(&releases).Error; err != nil {
		l.errs = append(l.errs, fmt.Errorf("querying GA releases: %w", err))
		return
	}
	for _, release := range releases {
		if release.GADate == nil {
			continue
		}
		if !l.force && release.LoadedGADate != nil && *release.LoadedGADate == *release.GADate {
			continue
		}
		start := utils.GAWindowStart(*release.GADate, 90).In(time.UTC)
		// Cleanup can remove the partition on the cutoff day.
		expired := !start.After(cutoff)
		if expired && !l.force {
			log.WithFields(log.Fields{"release": release.Release, "window_start": start, "retention_cutoff": cutoff}).Info("skipping GA snapshot; raw window is outside retention")
			continue
		}
		if err := l.loadRelease(release.Release, *release.GADate, start); err != nil {
			if errors.Is(err, db.ErrGARawWindowIncomplete) {
				if expired {
					log.WithFields(log.Fields{"release": release.Release, "window_start": start, "retention_cutoff": cutoff}).Info("skipping GA snapshot; expired raw window is incomplete")
					continue
				}
				if release.LoadedGADate != nil && *release.LoadedGADate == *release.GADate {
					log.WithField("release", release.Release).Info("preserving GA snapshot; raw window is incomplete")
					continue
				}
			}
			l.errs = append(l.errs, fmt.Errorf("release %s: %w", release.Release, err))
		}
	}
}

func (l *GATestStatusLoader) loadRelease(release string, gaDate civil.Date, start time.Time) error {
	end := utils.GAWindowEnd(gaDate).In(time.UTC)
	if err := l.dbc.DB.WithContext(l.ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateGARawData(tx, release, gaDate, start, end); err != nil {
			return err
		}
		if err := reaggregateGASnapshot(tx, release, gaDate, end); err != nil {
			return err
		}
		return recordGASnapshotLoaded(tx, release, gaDate)
	}); err != nil {
		return err
	}
	log.WithField("release", release).Info("GA snapshot refreshed from PostgreSQL")
	return nil
}

// validateGARawData prevents a partial raw window from replacing an existing snapshot.
func validateGARawData(tx *gorm.DB, release string, gaDate civil.Date, start, end time.Time) error {
	var rd models.ReleaseDefinition
	if err := tx.Where("release = ?", release).First(&rd).Error; err != nil {
		return fmt.Errorf("reading GA release: %w", err)
	}
	if rd.GADate == nil || *rd.GADate != gaDate {
		return fmt.Errorf("GA date changed during refresh")
	}
	if err := db.CheckGARawWindow(tx, release, start, end); err != nil {
		return err
	}
	var hasRows bool
	if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM prow_job_run_tests
        WHERE prow_job_run_release = ? AND prow_job_run_timestamp >= ? AND prow_job_run_timestamp < ? AND deleted_at IS NULL)`, release, start, end).Scan(&hasRows).Error; err != nil {
		return fmt.Errorf("checking raw rows: %w", err)
	}
	if !hasRows {
		return fmt.Errorf("no raw test rows in the full GA window; snapshot preserved")
	}
	return nil
}

// reaggregateGASnapshot replaces all configured windows after validation succeeds.
func reaggregateGASnapshot(tx *gorm.DB, release string, gaDate civil.Date, end time.Time) error {
	if err := tx.Exec("DELETE FROM prow_ga_raw_test_data WHERE release = ?", release).Error; err != nil {
		return fmt.Errorf("removing old GA snapshot: %w", err)
	}
	var inserted int64
	for _, days := range utils.GAWindows {
		windowStart := utils.GAWindowStart(gaDate, days).In(time.UTC)
		result := tx.Exec(`INSERT INTO prow_ga_raw_test_data
            (release, window_days, test_id, prow_job_id, suite_id, runs, passes, failures, flakes)
            SELECT ?, ?, t.test_id, r.prow_job_id, COALESCE(t.suite_id, 0), COUNT(*),
                COUNT(*) FILTER (WHERE t.status = 1),
                COUNT(*) FILTER (WHERE t.status IS DISTINCT FROM 1 AND t.status IS DISTINCT FROM 13),
                COUNT(*) FILTER (WHERE t.status = 13)
            FROM prow_job_run_tests t JOIN prow_job_runs r
              ON r.id = t.prow_job_run_id AND r.prow_job_release = t.prow_job_run_release
             AND r.timestamp = t.prow_job_run_timestamp
            WHERE t.prow_job_run_release = ? AND t.prow_job_run_timestamp >= ? AND t.prow_job_run_timestamp < ?
              AND t.deleted_at IS NULL AND r.deleted_at IS NULL
              AND (r.labels IS NULL OR NOT r.labels @> ARRAY['InfraFailure'])
            GROUP BY t.test_id, r.prow_job_id, COALESCE(t.suite_id, 0)`, release, days, release, windowStart, end)
		if result.Error != nil {
			return fmt.Errorf("aggregating %d-day GA snapshot: %w", days, result.Error)
		}
		inserted += result.RowsAffected
	}
	if inserted == 0 {
		return fmt.Errorf("no eligible raw test rows in the GA window; snapshot preserved")
	}
	return nil
}

// recordGASnapshotLoaded marks the release loaded only after its snapshot is complete.
func recordGASnapshotLoaded(tx *gorm.DB, release string, gaDate civil.Date) error {
	result := tx.Model(&models.ReleaseDefinition{}).Where("release = ? AND ga_date = ?", release, gaDate).
		Update("ga_data_loaded_date", gaDate)
	if result.Error != nil {
		return fmt.Errorf("recording GA snapshot: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("release or GA date changed during refresh")
	}
	return nil
}
