package componentreadiness

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/regressiontracker"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	testRegressionsTable = "test_regressions"

	// regressionHysteresisDays is the number of days a closed regression can be closed but still
	// reused if the test appears regressed again in that timeframe. It allows us to reuse the regression
	// record, including its start date, if the regression is coming and going.
	regressionHysteresisDays = 5
)

// RegressionStore is an underlying interface for where we store/load data on open test regressions.
type RegressionStore interface {
	// ListCurrentRegressionsForRelease returns *all* regressions for the given release
	ListCurrentRegressionsForRelease(release string) ([]*models.TestRegression, error)
	OpenRegression(view crview.View, newRegressedTest crtype.ReportTestSummary) (*models.TestRegression, error)
	UpdateRegression(reg *models.TestRegression) error
	// ResolveTriages sets the resolution time on any triages that no longer have active regressions
	ResolveTriages() error
	// ForceCloseRegressions closes the open regressions associated with the given resolved triage that
	// existed at its resolution time, marking them force closed so they are excluded from the reuse window
	// and not reopened for unrelated failures. Returns ErrTriageNotResolved if the triage is not resolved.
	// It is idempotent.
	ForceCloseRegressions(triageID uint, closedBy, reason string) (*ForceCloseResult, error)
	// ForceClosePreview returns a dry-run of what ForceCloseRegressions would do for the given triage,
	// without modifying anything. Returns ErrTriageNotResolved if the triage is not resolved.
	ForceClosePreview(triageID uint) (*ForceClosePreview, error)
	// MergeJobRuns upserts job runs for a regression, adding new ones and skipping duplicates.
	MergeJobRuns(regressionID uint, jobRuns []models.RegressionJobRun) error
	// UpsertRegressionView records that a regression was observed in a view, setting active=true.
	UpsertRegressionView(regressionID uint, viewName string) error
	// DeactivateRolledOffViews sets active=false on regression_views rows for regressions that have rolled off a view.
	DeactivateRolledOffViews(regressionIDs []uint, activeViewMap map[uint][]string) error
	// SyncTriageSymptoms upserts symptom associations for triages based on regression job run data.
	SyncTriageSymptoms(regressions []*models.TestRegression) error
}

type PostgresRegressionStore struct {
	dbc        *db.DB
	jiraClient *jira.Client
}

func NewPostgresRegressionStore(dbc *db.DB, jiraClient *jira.Client) RegressionStore {
	return &PostgresRegressionStore{dbc: dbc, jiraClient: jiraClient}
}

func (prs *PostgresRegressionStore) ListCurrentRegressionsForRelease(release string) ([]*models.TestRegression, error) {
	// List open regressions (no closed date), or those that closed within the last few days. This is to prevent flapping
	// and return more accurate opened dates when a test is falling in / out of the report.
	regressions := make([]*models.TestRegression, 0)
	// Force closed regressions are excluded from the reuse window even if they closed recently, so they
	// are not reopened for unrelated failures (TRT-2895).
	q := prs.dbc.DB.Table(testRegressionsTable).
		Where("release = ?", release).
		Where("closed IS NULL OR (closed > ? AND force_closed = false)", time.Now().Add(-regressionHysteresisDays*24*time.Hour))
	res := q.Scan(&regressions)
	return regressions, res.Error
}

func (prs *PostgresRegressionStore) OpenRegression(view crview.View, newRegressedTest crtype.ReportTestSummary) (*models.TestRegression, error) {
	variants := utils.VariantsMapToStringSlice(newRegressedTest.Variants)

	newRegression := &models.TestRegression{
		Release:     view.SampleRelease.Name,
		TestID:      newRegressedTest.TestID,
		TestName:    newRegressedTest.TestName,
		Opened:      time.Now(),
		Variants:    variants,
		MaxFailures: newRegressedTest.SampleStats.FailureCount,
	}

	// Store the base release
	// so we can generate accurate test_details API links.
	// Start with the view's base release, but if the test got a base release override to a prior release, we use that instead.
	newRegression.BaseRelease = view.BaseRelease.Name
	if newRegressedTest.BaseStats != nil {
		newRegression.BaseRelease = newRegressedTest.BaseStats.Release
	}

	newRegression.CrossCompare = len(view.VariantOptions.VariantCrossCompare) > 0
	newRegression.Capability = newRegressedTest.Capability
	newRegression.Component = newRegressedTest.Component

	if newRegressedTest.LastFailure != nil {
		newRegression.LastFailure = sql.NullTime{Valid: true, Time: *newRegressedTest.LastFailure}
	}
	res := prs.dbc.DB.Create(newRegression)
	if res.Error != nil {
		return &models.TestRegression{}, res.Error
	}
	log.Infof("opened a new regression: %v", newRegression)
	return newRegression, nil

}

func (prs *PostgresRegressionStore) UpdateRegression(reg *models.TestRegression) error {
	res := prs.dbc.DB.Save(&reg)
	return res.Error
}

func (prs *PostgresRegressionStore) MergeJobRuns(regressionID uint, jobRuns []models.RegressionJobRun) error {
	for i := range jobRuns {
		jobRuns[i].RegressionID = regressionID
		res := prs.dbc.DB.
			Where("regression_id = ? AND prow_job_run_id = ?", regressionID, jobRuns[i].ProwJobRunID).
			Assign(models.RegressionJobRun{
				JobLabels:   jobRuns[i].JobLabels,
				JobSymptoms: jobRuns[i].JobSymptoms,
			}).
			FirstOrCreate(&jobRuns[i])
		if res.Error != nil {
			return fmt.Errorf("error merging job run %s for regression %d: %w",
				jobRuns[i].ProwJobRunID, regressionID, res.Error)
		}
	}
	return nil
}

// SyncTriageSymptoms upserts triage_symptoms junction rows by doing a full recount of
// symptoms across each regression's job runs. The resulting job_run_count is replaced
// (not incremented), making the operation idempotent and safe to call on every loader run.
func (prs *PostgresRegressionStore) SyncTriageSymptoms(regressions []*models.TestRegression) error {
	if len(regressions) == 0 {
		return nil
	}

	regIDs := make([]uint, len(regressions))
	for i, r := range regressions {
		regIDs[i] = r.ID
	}

	var regs []models.TestRegression
	res := prs.dbc.DB.
		Preload("Triages").
		Preload("JobRuns").
		Where("id IN ?", regIDs).
		Find(&regs)
	if res.Error != nil {
		return fmt.Errorf("error loading regressions for symptom sync: %w", res.Error)
	}

	for _, reg := range regs {
		if len(reg.Triages) == 0 {
			continue
		}
		symptomCounts := map[string]int{}
		for _, jr := range reg.JobRuns {
			seen := sets.New[string]()
			for _, symptom := range jr.JobSymptoms {
				if symptom != "" && !seen.Has(symptom) {
					seen.Insert(symptom)
					symptomCounts[symptom]++
				}
			}
		}
		for _, triage := range reg.Triages {
			for symptomID, count := range symptomCounts {
				if err := prs.dbc.DB.Exec(
					`INSERT INTO triage_symptoms (triage_id, symptom_id, regression_id, job_run_count)
					 VALUES (?, ?, ?, ?)
					 ON CONFLICT (triage_id, symptom_id, regression_id) DO UPDATE
					 SET job_run_count = EXCLUDED.job_run_count`,
					triage.ID, symptomID, reg.ID, count).Error; err != nil {
					return fmt.Errorf("error syncing symptom %s to triage %d regression %d: %w",
						symptomID, triage.ID, reg.ID, err)
				}
			}
		}
	}
	return nil
}

func (prs *PostgresRegressionStore) UpsertRegressionView(regressionID uint, viewName string) error {
	res := prs.dbc.DB.Exec(
		`INSERT INTO regression_views (test_regression_id, view_name, active, opened_at)
		 VALUES (?, ?, true, NOW())
		 ON CONFLICT (test_regression_id, view_name) DO UPDATE
		 SET active = true,
		     opened_at = CASE WHEN regression_views.active = false THEN NOW() ELSE regression_views.opened_at END,
		     closed_at = NULL`,
		regressionID, viewName)
	return res.Error
}

func (prs *PostgresRegressionStore) DeactivateRolledOffViews(regressionIDs []uint, activeViewMap map[uint][]string) error {
	if len(regressionIDs) == 0 {
		return nil
	}

	for _, regID := range regressionIDs {
		q := prs.dbc.DB.Model(&models.RegressionView{}).
			Where("test_regression_id = ? AND active = true", regID)
		if activeViews := activeViewMap[regID]; len(activeViews) > 0 {
			q = q.Where("view_name NOT IN ?", activeViews)
		}
		if res := q.Updates(map[string]interface{}{"active": false, "closed_at": time.Now()}); res.Error != nil {
			return fmt.Errorf("error deactivating rolled-off views for regression %d: %w", regID, res.Error)
		}
	}
	return nil
}

// ResolveTriages sets the resolution time on any triages that no longer have active regressions
// It only does so when all the regressions have been closed for at least regressionHysteresisDays (5) days
func (prs *PostgresRegressionStore) ResolveTriages() error {
	hysteresisTime := time.Now().Add(-regressionHysteresisDays * 24 * time.Hour)
	var triagesToResolve []models.Triage
	subQuery := prs.dbc.DB.Table("triage_regressions tr").
		Joins("JOIN test_regressions r ON tr.test_regression_id = r.id").
		Where("tr.triage_id = triages.id").
		Where("r.closed IS NULL OR (r.closed > ? AND r.force_closed = false)", hysteresisTime).
		Select("1")

	res := prs.dbc.DB.Table("triages").
		Where("resolved IS NULL").
		Where("NOT EXISTS (?)", subQuery).
		Preload("Regressions").
		Find(&triagesToResolve)

	if res.Error != nil {
		return fmt.Errorf("error finding triages to resolve: %v", res.Error)
	}

	log.Infof("Found %d triages to resolve", len(triagesToResolve))

	for _, triage := range triagesToResolve {
		var mostRecentClosedRegression models.TestRegression

		// Find the latest, closed regression in order to get the resolution time
		regQuery := prs.dbc.DB.Table("test_regressions").
			Joins("JOIN triage_regressions ON triage_regressions.test_regression_id = test_regressions.id").
			Where("triage_regressions.triage_id = ?", triage.ID).
			Where("test_regressions.closed IS NOT NULL").
			Order("test_regressions.closed DESC").
			Limit(1)

		res := regQuery.First(&mostRecentClosedRegression)
		if res.Error != nil {
			log.WithError(res.Error).Errorf("error finding most recent closed regression for triage %d", triage.ID)
			continue
		}

		triage.Resolved = mostRecentClosedRegression.Closed
		triage.ResolutionReason = models.RegressionsRolledOff
		dbWithContext := prs.dbc.DB.WithContext(context.WithValue(context.Background(), models.CurrentUserKey, "regression-tracker"))
		res = dbWithContext.Save(&triage)
		if res.Error != nil {
			log.WithError(res.Error).Errorf("error resolving triage %d", triage.ID)
			continue
		}

		ReportTriageResolved(prs.jiraClient, triage)
		log.Infof("Resolved triage %d with resolution time %v", triage.ID, triage.Resolved.Time)
	}

	return nil
}

// ErrTriageNotResolved is returned by force close operations when the triage has not been resolved.
// Force closing needs a resolution time to scope which regressions to close and when to close them.
var ErrTriageNotResolved = errors.New("triage is not resolved")

// ForceCloseResult summarizes the outcome of a force close operation. It is returned by the API
// so callers know which regressions were closed and at what time.
type ForceCloseResult struct {
	// ClosedRegressionIDs are the IDs of the regressions that were open and got closed by this call.
	// On an idempotent repeat call (regressions already closed) this will be empty.
	ClosedRegressionIDs []uint `json:"closed_regression_ids"`
	// Timestamp is the closed time applied to the regressions (the triage's resolution time).
	Timestamp time.Time `json:"timestamp"`
}

// RegressionFailureGap describes the failure timing around a triage's resolution for a single regression.
// A non-nil FirstFailureAfterResolution means the test kept failing after the claimed resolution, a signal
// the triage may have been resolved prematurely and force closing warrants a closer look.
type RegressionFailureGap struct {
	// LastFailureBeforeResolution is the most recent failing job run at or before the resolution time.
	LastFailureBeforeResolution *time.Time `json:"last_failure_before_resolution,omitempty"`
	// FirstFailureAfterResolution is the earliest failing job run after the resolution time, if any.
	FirstFailureAfterResolution *time.Time `json:"first_failure_after_resolution,omitempty"`
}

// ForceClosePreviewRegression describes a single regression in a force close preview, including the failure
// gap around the triage's resolution so a user can judge whether force closing is appropriate.
type ForceClosePreviewRegression struct {
	RegressionID uint           `json:"regression_id"`
	TestName     string         `json:"test_name"`
	Variants     pq.StringArray `json:"variants"`
	Opened       time.Time      `json:"opened"`
	// Closed is set when the regression is already closed.
	Closed *time.Time `json:"closed,omitempty"`
	// RegressionFailureGap is embedded so its fields are promoted into this object's JSON.
	RegressionFailureGap
}

// ForceClosePreview is the dry-run result for force closing a triage's regressions. WouldClose lists the
// open regressions that existed at the resolution time and would be closed; WouldNotClose lists regressions
// that opened after the resolution time and would be left untouched.
type ForceClosePreview struct {
	TriageID      uint                          `json:"triage_id"`
	Resolved      time.Time                     `json:"resolved"`
	WouldClose    []ForceClosePreviewRegression `json:"would_close"`
	WouldNotClose []ForceClosePreviewRegression `json:"would_not_close"`
}

// ForceCloseRegressions closes the open regressions associated with the given resolved triage that existed
// at its resolution time (opened at or before triage.Resolved), marking them force closed so they are
// excluded from the regression reuse window (regressionHysteresisDays) and never reopened for unrelated
// failures (TRT-2895). Each regression is closed at the triage's resolution time and records who force
// closed it and why, directly on the regression row. The triage must be resolved; otherwise
// ErrTriageNotResolved is returned. It is idempotent: already-closed regressions are left untouched.
func (prs *PostgresRegressionStore) ForceCloseRegressions(triageID uint, closedBy, reason string) (*ForceCloseResult, error) {
	result := &ForceCloseResult{}

	err := prs.dbc.DB.Transaction(func(tx *gorm.DB) error {
		var triage models.Triage
		if err := tx.Preload("Regressions").First(&triage, triageID).Error; err != nil {
			return fmt.Errorf("error loading triage %d for force close: %w", triageID, err)
		}
		if !triage.Resolved.Valid {
			return ErrTriageNotResolved
		}
		closeTime := triage.Resolved.Time
		result.Timestamp = closeTime

		for i := range triage.Regressions {
			reg := &triage.Regressions[i]
			// Only close regressions that existed at the resolution time and are still open. This scopes the
			// action to what the triage actually resolved, keeps it idempotent, and preserves the original
			// closed time of any already-closed regression.
			if reg.Closed.Valid || reg.Opened.After(closeTime) {
				continue
			}
			// Update only the affected columns to avoid rewriting the many2many triage associations.
			updates := map[string]interface{}{
				"closed":                    sql.NullTime{Valid: true, Time: closeTime},
				"force_closed":              true,
				"force_closed_by":           closedBy,
				"force_closed_reason":       reason,
				"force_closed_by_triage_id": triageID,
			}
			if err := tx.Model(&models.TestRegression{}).Where("id = ?", reg.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("error force closing regression %d: %w", reg.ID, err)
			}
			result.ClosedRegressionIDs = append(result.ClosedRegressionIDs, reg.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	log.WithField("triageID", triageID).WithField("closedBy", closedBy).
		WithField("closedRegressions", len(result.ClosedRegressionIDs)).Info("force closed regressions for triage")
	return result, nil
}

// queryRegressionFailureGap computes, for a single regression, the last failing job run at or before
// resolutionTime and the first failing job run after resolutionTime using the regression_job_runs table.
// Either bound may be nil when no matching failing run exists.
func (prs *PostgresRegressionStore) queryRegressionFailureGap(regressionID uint, resolutionTime time.Time) (RegressionFailureGap, error) {
	gap := RegressionFailureGap{}

	var lastBefore sql.NullTime
	lastRow := prs.dbc.DB.Table("regression_job_runs").
		Where("regression_id = ? AND start_time <= ? AND test_failed = true", regressionID, resolutionTime).
		Select("MAX(start_time)").Row()
	if err := lastRow.Scan(&lastBefore); err != nil {
		return gap, fmt.Errorf("error querying last failure before resolution for regression %d: %w", regressionID, err)
	}
	if lastBefore.Valid {
		t := lastBefore.Time
		gap.LastFailureBeforeResolution = &t
	}

	var firstAfter sql.NullTime
	firstRow := prs.dbc.DB.Table("regression_job_runs").
		Where("regression_id = ? AND start_time > ? AND test_failed = true", regressionID, resolutionTime).
		Select("MIN(start_time)").Row()
	if err := firstRow.Scan(&firstAfter); err != nil {
		return gap, fmt.Errorf("error querying first failure after resolution for regression %d: %w", regressionID, err)
	}
	if firstAfter.Valid {
		t := firstAfter.Time
		gap.FirstFailureAfterResolution = &t
	}

	return gap, nil
}

// ForceClosePreview returns a dry-run of what ForceCloseRegressions would do for the given triage without
// modifying anything. The triage must be resolved; otherwise ErrTriageNotResolved is returned. Each entry
// includes the failure gap around the resolution time so callers can spot regressions that kept failing.
func (prs *PostgresRegressionStore) ForceClosePreview(triageID uint) (*ForceClosePreview, error) {
	var triage models.Triage
	if err := prs.dbc.DB.Preload("Regressions").First(&triage, triageID).Error; err != nil {
		return nil, fmt.Errorf("error loading triage %d for force close preview: %w", triageID, err)
	}
	if !triage.Resolved.Valid {
		return nil, ErrTriageNotResolved
	}
	resolved := triage.Resolved.Time
	preview := &ForceClosePreview{TriageID: triageID, Resolved: resolved}

	for i := range triage.Regressions {
		reg := &triage.Regressions[i]
		gap, err := prs.queryRegressionFailureGap(reg.ID, resolved)
		if err != nil {
			return nil, err
		}
		entry := ForceClosePreviewRegression{
			RegressionID:         reg.ID,
			TestName:             reg.TestName,
			Variants:             reg.Variants,
			Opened:               reg.Opened,
			RegressionFailureGap: gap,
		}
		if reg.Closed.Valid {
			c := reg.Closed.Time
			entry.Closed = &c
		}
		switch {
		case !reg.Closed.Valid && !reg.Opened.After(resolved):
			// Open and existed at the resolution time: this is what force close would close.
			preview.WouldClose = append(preview.WouldClose, entry)
		case reg.Opened.After(resolved):
			// Opened after the resolution time: force close would leave it untouched.
			preview.WouldNotClose = append(preview.WouldNotClose, entry)
		}
		// Already-closed regressions that opened before resolution are omitted: force closing would not
		// change them.
	}
	return preview, nil
}

// SyncRegressionsForReport compares regressed tests from a component report against known
// regressions in the database, opening new ones, reopening recently closed ones, and updating
// stats on existing ones. Returns the list of active regressions after sync.
func SyncRegressionsForReport(
	backend RegressionStore,
	view crview.View,
	rLog *log.Entry,
	report *crtype.ComponentReport,
) ([]*models.TestRegression, error) {
	regressions, err := backend.ListCurrentRegressionsForRelease(view.SampleRelease.Name)
	if err != nil {
		return nil, err
	}
	rLog.Infof("loaded %d regressions from db for release %s", len(regressions), view.SampleRelease.Name)

	// All regressed tests, both triaged and not:
	var allRegressedTests []crtype.ReportTestSummary
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			allRegressedTests = append(allRegressedTests, col.RegressedTests...)
		}
	}

	var openedRegs, reopenedRegs, ongoingRegs, statsUpdatedRegs int
	var activeRegressions []*models.TestRegression // all the matches we found, and new regressions opened, used to determine what had no match
	rLog.Infof("syncing %d open regressions", len(allRegressedTests))
	for _, regTest := range allRegressedTests {
		crossCompare := len(view.VariantOptions.VariantCrossCompare) > 0
		if openReg := regressiontracker.FindOpenRegression(view.SampleRelease.Name, regTest.TestID, crossCompare, regTest.Variants, regressions); openReg != nil {

			// Check if we need to add new variants to the regression found via subset matching.
			// This allows regressions to be split by new variant dimensions when db_column_groupby is modified.
			existingVariantMap := make(map[string]bool)
			for _, v := range openReg.Variants {
				existingVariantMap[v] = true
			}

			var newVariants []string
			for key, value := range regTest.Variants {
				variantStr := fmt.Sprintf("%s:%s", key, value)
				if !existingVariantMap[variantStr] {
					newVariants = append(newVariants, variantStr)
					openReg.Variants = append(openReg.Variants, variantStr)
				}
			}

			if len(newVariants) > 0 {
				rLog.Infof("updating regression %d to include new variants: %v", openReg.ID, newVariants)
				if err := backend.UpdateRegression(openReg); err != nil {
					return nil, fmt.Errorf("failed to update regression %d with new variants: %w", openReg.ID, err)
				}
			}

			// Update any tracking params on the regression if we see better values:
			var modifiedRegression bool
			if regTest.SampleStats.FailureCount > openReg.MaxFailures {
				openReg.MaxFailures = regTest.SampleStats.FailureCount
				modifiedRegression = true
			}
			if regTest.LastFailure != nil {
				if !openReg.LastFailure.Valid || regTest.LastFailure.After(openReg.LastFailure.Time) {
					openReg.LastFailure = sql.NullTime{Valid: true, Time: *regTest.LastFailure}
					modifiedRegression = true
				}
			}

			// BaseRelease was added to test_regressions later, this block allows us to set it for any pre-existing
			// regressions as soon as the reg tracker runs.
			// TODO: remove this block and make the field non-nullable once the db is updated
			baseRelease := view.BaseRelease.Name
			if regTest.BaseStats != nil {
				baseRelease = regTest.BaseStats.Release
			}
			if baseRelease != openReg.BaseRelease {
				openReg.BaseRelease = baseRelease
				modifiedRegression = true
			}

			// Technically component and capability could get remapped during the time the regression is open,
			// and we need this to roll out the storing of these fields initially:
			if regTest.Component != openReg.Component {
				openReg.Component = regTest.Component
				modifiedRegression = true
			}
			if regTest.Capability != openReg.Capability {
				openReg.Capability = regTest.Capability
				modifiedRegression = true
			}

			if modifiedRegression {
				statsUpdatedRegs++
				err := backend.UpdateRegression(openReg)
				if err != nil {
					rLog.WithError(err).Errorf("error updating regression: %v", openReg)
					return nil, fmt.Errorf("error updating regression: %v: %w", openReg, err)
				}
			}

			if openReg.Closed.Valid {
				// if the regression returned has a closedRegs date, we found a recently closedRegs
				// regression for this test. We'll re-use it to limit churn as sometimes tests may drop
				// in / out of the report depending on the data available in the sample/basis.
				rLog.Infof("re-opening existing regression: %v", openReg)
				reopenedRegs++
				openReg.Closed = sql.NullTime{Valid: false}
				err := backend.UpdateRegression(openReg)
				if err != nil {
					rLog.WithError(err).Errorf("error re-opening regression: %v", openReg)
					return nil, fmt.Errorf("error re-opening regression: %v: %w", openReg, err)
				}
			} else {
				// Still consider untouched even if we bumped the max failures count
				ongoingRegs++
				rLog.WithFields(log.Fields{
					"test": regTest.TestName,
				}).Debugf("reusing already opened regression: %v", openReg)
			}
			activeRegressions = append(activeRegressions, openReg)
		} else {
			openedRegs++
			rLog.Infof("opening new regression: %v", regTest)
			// Open a new regression:
			newReg, err := backend.OpenRegression(view, regTest)
			if err != nil {
				rLog.WithError(err).Errorf("error opening new regression for: %v", regTest)
				return nil, fmt.Errorf("error opening new regression: %v: %w", regTest, err)
			}
			activeRegressions = append(activeRegressions, newReg)
			rLog.Infof("new regression opened with id: %d", newReg.ID)
		}
	}

	rLog.Infof("regression tracking sync completed: opened=%d, reopened=%d, ongoing=%d, statsUpdated=%d",
		openedRegs, reopenedRegs, ongoingRegs, statsUpdatedRegs)

	return activeRegressions, nil
}

// FailedJobRunsFromTestDetails extracts sample job runs where the test failed
// from a test details report and converts them to RegressionJobRun records.
func FailedJobRunsFromTestDetails(report testdetails.Report) []models.RegressionJobRun {
	var jobRuns []models.RegressionJobRun
	for _, analysis := range report.Analyses {
		for _, jobStat := range analysis.JobStats {
			for _, run := range jobStat.SampleJobRunStats {
				if run.TestStats.FailureCount == 0 {
					continue
				}
				jobRun := models.RegressionJobRun{
					ProwJobRunID: run.JobRunID,
					ProwJobName:  jobStat.SampleJobName,
					ProwJobURL:   run.JobURL,
					StartTime:    run.StartTime.In(time.UTC),
					TestFailures: run.TestFailures,
					JobLabels:    pq.StringArray(run.JobLabels),
					JobSymptoms:  pq.StringArray(run.JobSymptoms),
				}
				jobRuns = append(jobRuns, jobRun)
			}
		}
	}
	return jobRuns
}
