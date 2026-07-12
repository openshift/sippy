package cumulativesummary

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
)

type variantStore interface {
	MaxVariantCumulativeSummaryDate() (*civil.Date, error)
	MaxCumulativeSummaryDate() (*civil.Date, error)
	Releases() ([]string, error)
	UpdateVariantDateForRelease(date civil.Date, release string) error
	DetectVariantChanges() ([]uint, error)
	ScopedRebuildVariants(changedProwJobIDs []uint) error
	UpdateVCIDMapping() error
	VCIDMappingPopulated() (bool, error)
}

// RefreshVariantCumulativeSummaries groups test_cumulative_summaries by variant_combination_id
// into the variant_cumulative_summaries table. Each date's update groups one
// date's test_cumulative_summaries and inserts. Variant changes are detected via
// vcid_mappings and trigger scoped rebuilds.
func RefreshVariantCumulativeSummaries(dbc *db.DB, earliestChanged civil.Date) error {
	return doVariantRefresh(&pgVariantStore{dbc: dbc}, earliestChanged)
}

// BackfillVariant processes an explicit date range without automatic date detection.
func BackfillVariant(dbc *db.DB, startDate, endDate civil.Date) error {
	store := &pgVariantStore{dbc: dbc}
	return updateVariantDates(store, startDate, endDate)
}

func doVariantRefresh(store variantStore, earliestChanged civil.Date) error {
	loadStart := time.Now()
	log.Info("refreshing variant cumulative summaries")

	populated, err := store.VCIDMappingPopulated()
	if err != nil {
		return fmt.Errorf("checking VCID mapping: %w", err)
	}
	if populated {
		changes, err := store.DetectVariantChanges()
		if err != nil {
			return fmt.Errorf("detecting variant changes: %w", err)
		}
		if len(changes) > 0 {
			log.WithField("count", len(changes)).Info("variant changes detected, doing scoped rebuild")
			if err := store.ScopedRebuildVariants(changes); err != nil {
				return fmt.Errorf("scoped rebuild for variant changes: %w", err)
			}
		}
	} else {
		log.Info("VCID mapping empty (first run), skipping variant detection")
	}

	maxVariantCumulativeSummaryDate, err := store.MaxVariantCumulativeSummaryDate()
	if err != nil {
		return fmt.Errorf("checking max variant cumulative summary date: %w", err)
	}

	maxCumulativeSummaryDate, err := store.MaxCumulativeSummaryDate()
	if err != nil {
		return fmt.Errorf("checking max cumulative summary date: %w", err)
	}
	if maxCumulativeSummaryDate == nil {
		log.Info("no cumulative summaries found, nothing to group")
		return nil
	}

	startDate := resolveStartDate(earliestChanged, maxVariantCumulativeSummaryDate, civil.DateOf(time.Now().UTC()))

	endDate := *maxCumulativeSummaryDate

	if err := updateVariantDates(store, startDate, endDate); err != nil {
		return err
	}

	if err := store.UpdateVCIDMapping(); err != nil {
		return fmt.Errorf("updating VCID mapping: %w", err)
	}

	log.WithField("elapsed", time.Since(loadStart)).Info("variant cumulative summary refresh complete")
	return nil
}

func updateVariantDates(store variantStore, startDate, endDate civil.Date) error {
	loadStart := time.Now()
	days := endDate.DaysSince(startDate) + 1

	releases, err := store.Releases()
	if err != nil {
		return fmt.Errorf("querying releases: %w", err)
	}

	log.WithFields(log.Fields{
		"start":    startDate,
		"end":      endDate,
		"days":     days,
		"releases": len(releases),
	}).Info("updating variant cumulative summaries")

	for date := startDate; !date.After(endDate); date = date.AddDays(1) {
		updateStart := time.Now()
		for _, release := range releases {
			if err := store.UpdateVariantDateForRelease(date, release); err != nil {
				return fmt.Errorf("updating variant cumulative summaries for %s release %s: %w", date, release, err)
			}
		}
		log.WithFields(log.Fields{
			"date":    date,
			"elapsed": time.Since(updateStart),
		}).Info("updated variant cumulative summaries for date")
	}

	log.WithField("elapsed", time.Since(loadStart)).Info("variant cumulative summary update complete")
	return nil
}

type pgVariantStore struct {
	dbc *db.DB
}

func (s *pgVariantStore) MaxVariantCumulativeSummaryDate() (*civil.Date, error) {
	var d *civil.Date
	err := s.dbc.DB.Table("variant_cumulative_summaries").
		Select("MAX(date)").Row().Scan(&d)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (s *pgVariantStore) MaxCumulativeSummaryDate() (*civil.Date, error) {
	var d *civil.Date
	err := s.dbc.DB.Table("test_cumulative_summaries").
		Select("MAX(date)").Row().Scan(&d)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (s *pgVariantStore) Releases() ([]string, error) {
	var releases []string
	err := s.dbc.DB.Table("release_definitions").
		Order("major DESC, minor DESC").
		Pluck("release", &releases).Error
	return releases, err
}

func (s *pgVariantStore) UpdateVariantDateForRelease(date civil.Date, release string) error {
	return s.dbc.DB.Exec(`
		INSERT INTO variant_cumulative_summaries (date, test_id, suite_id, variant_combination_id, release,
		                                 prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs)
		SELECT ps.date, ps.test_id, ps.suite_id, pj.variant_combination_id, ps.release,
			SUM(ps.prefix_sum_successes)::bigint,
			SUM(ps.prefix_sum_failures)::bigint,
			SUM(ps.prefix_sum_flakes)::bigint,
			SUM(ps.prefix_sum_runs)::bigint
		FROM test_cumulative_summaries ps
		JOIN prow_jobs pj ON ps.prow_job_id = pj.id
		WHERE ps.date = ? AND ps.release = ?
		  AND pj.variant_combination_id IS NOT NULL
		GROUP BY ps.date, ps.test_id, ps.suite_id, pj.variant_combination_id, ps.release
		ON CONFLICT (date, release, test_id, suite_id, variant_combination_id)
		DO UPDATE SET
			prefix_sum_successes = EXCLUDED.prefix_sum_successes,
			prefix_sum_failures = EXCLUDED.prefix_sum_failures,
			prefix_sum_flakes = EXCLUDED.prefix_sum_flakes,
			prefix_sum_runs = EXCLUDED.prefix_sum_runs
		WHERE (variant_cumulative_summaries.prefix_sum_successes, variant_cumulative_summaries.prefix_sum_failures,
		       variant_cumulative_summaries.prefix_sum_flakes, variant_cumulative_summaries.prefix_sum_runs)
		   IS DISTINCT FROM
		      (EXCLUDED.prefix_sum_successes, EXCLUDED.prefix_sum_failures,
		       EXCLUDED.prefix_sum_flakes, EXCLUDED.prefix_sum_runs)
	`, date, release).Error
}

func (s *pgVariantStore) VCIDMappingPopulated() (bool, error) {
	var exists bool
	err := s.dbc.DB.Raw("SELECT EXISTS(SELECT 1 FROM vcid_mappings LIMIT 1)").Scan(&exists).Error
	return exists, err
}

func (s *pgVariantStore) DetectVariantChanges() ([]uint, error) {
	var changedIDs []uint
	err := s.dbc.DB.Raw(`
		SELECT pj.id FROM prow_jobs pj
		LEFT JOIN vcid_mappings m ON pj.id = m.prow_job_id
		WHERE m.prow_job_id IS NULL
		   OR m.variant_combination_id IS DISTINCT FROM pj.variant_combination_id
	`).Pluck("id", &changedIDs).Error
	return changedIDs, err
}

func (s *pgVariantStore) ScopedRebuildVariants(changedProwJobIDs []uint) error {
	return s.dbc.DB.Exec(`
		WITH affected_entities AS (
			SELECT DISTINCT ps.test_id, ps.suite_id, ps.release
			FROM test_cumulative_summaries ps
			WHERE ps.prow_job_id IN (?)
		),
		deleted AS (
			DELETE FROM variant_cumulative_summaries vps
			USING affected_entities ae
			WHERE vps.test_id = ae.test_id
			  AND vps.suite_id = ae.suite_id
			  AND vps.release = ae.release
		)
		INSERT INTO variant_cumulative_summaries (date, test_id, suite_id, variant_combination_id, release,
		                                 prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs)
		SELECT ps.date, ps.test_id, ps.suite_id, pj.variant_combination_id, ps.release,
			SUM(ps.prefix_sum_successes)::bigint, SUM(ps.prefix_sum_failures)::bigint,
			SUM(ps.prefix_sum_flakes)::bigint, SUM(ps.prefix_sum_runs)::bigint
		FROM test_cumulative_summaries ps
		JOIN prow_jobs pj ON ps.prow_job_id = pj.id
		JOIN affected_entities ae ON ps.test_id = ae.test_id
			AND ps.suite_id = ae.suite_id AND ps.release = ae.release
		WHERE pj.variant_combination_id IS NOT NULL
		  AND ps.date >= CURRENT_DATE - 90
		GROUP BY ps.date, ps.test_id, ps.suite_id, pj.variant_combination_id, ps.release
	`, changedProwJobIDs).Error
}

func (s *pgVariantStore) UpdateVCIDMapping() error {
	return s.dbc.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("TRUNCATE vcid_mappings").Error; err != nil {
			return err
		}
		return tx.Exec(`
			INSERT INTO vcid_mappings (prow_job_id, variant_combination_id)
			SELECT id, variant_combination_id FROM prow_jobs
		`).Error
	})
}
