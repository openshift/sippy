package componentreadiness

import (
	"context"
	"database/sql"
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
	q := prs.dbc.DB.Table(testRegressionsTable).
		Where("release = ?", release).
		Where("closed IS NULL OR closed > ?", time.Now().Add(-regressionHysteresisDays*24*time.Hour))
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
		Where("r.closed IS NULL OR r.closed > ?", hysteresisTime).
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
	regressionIndex := regressiontracker.BuildRegressionIndex(regressions)
	rLog.Infof("syncing %d open regressions", len(allRegressedTests))
	crossCompare := len(view.VariantOptions.VariantCrossCompare) > 0
	for _, regTest := range allRegressedTests {
		if openReg := regressiontracker.FindOpenRegression(view.SampleRelease.Name, regTest.TestID, crossCompare, regTest.Variants, regressionIndex); openReg != nil {

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
