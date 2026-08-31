package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	gosort "sort"
	"strings"
	"time"

	bqlib "cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"google.golang.org/api/iterator"

	"github.com/hashicorp/go-version"
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/apis/openshift"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testidentification"
)

const (
	// maxFailuresToFullyAnalyze is a limit to the number of failures we'll attempt to
	// individually analyze, if you exceed this the job failure is classified as high risk.
	maxFailuresToFullyAnalyze = 20
)

// nonDeterministicRiskLevels indicate incomplete analysis and allow for fallback to other analysis methodologies name -> variant
var nonDeterministicRiskLevels = []int{apitype.FailureRiskLevelUnknown.Level, apitype.FailureRiskLevelIncompleteTests.Level, apitype.FailureRiskLevelMissingData.Level}

// JobsRunsReportFromDB renders a filtered summary of matching jobs using a
// two-phase approach: Phase 1 paginates from base tables (prow_job_runs +
// prow_jobs), Phase 2 enriches the page with test counts, test name arrays,
// pull request data, and annotations.
func JobsRunsReportFromDB(dbc *db.DB, filterOpts *filter.FilterOptions, release string, pagination *apitype.Pagination, reportEnd time.Time) (*apitype.PaginationResult, error) {
	if filterOpts.SortField != "" && isTestNameField(filterOpts.SortField) {
		return nil, &ValidationError{Message: fmt.Sprintf("sorting by %s is not supported", filterOpts.SortField)}
	}

	if len(release) == 0 {
		return nil, &ValidationError{Message: "release is required; use useCurrentRelease=true to resolve automatically"}
	}

	jobsResult := make([]apitype.JobRun, 0)
	lookback := reportEnd.Add(-90 * 24 * time.Hour)

	prColumns := sets.New[string]("pull_request_link", "pull_request_sha", "pull_request_org", "pull_request_repo", "pull_request_author")
	needs := analyzeJobRunFilters(filterOpts, prColumns)

	// Phase 1: build SELECT from base tables.
	selectSQL := jobRunsBaseSelect
	if needs.needsPRJoin() {
		selectSQL += `, pp.link AS pull_request_link, pp.sha AS pull_request_sha, pp.org AS pull_request_org, pp.repo AS pull_request_repo, pp.author AS pull_request_author`
	}

	dbQuery := dbc.DB.Table("prow_job_runs").
		Select(selectSQL).
		Joins("JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id")

	addPRJoin := func(q *gorm.DB) *gorm.DB {
		return q.
			Joins(`LEFT JOIN (SELECT DISTINCT ON(prow_job_run_id) prow_job_run_id, prow_pull_request_id FROM prow_job_run_prow_pull_requests WHERE prow_job_run_release = ? ORDER BY prow_job_run_id, prow_pull_request_id DESC) jrpp ON jrpp.prow_job_run_id = prow_job_runs.id`, release).
			Joins("LEFT JOIN prow_pull_requests pp ON pp.id = jrpp.prow_pull_request_id")
	}

	if needs.prJoinForFilter {
		dbQuery = addPRJoin(dbQuery)
	}

	q, err := applyJobRunFilters(dbQuery, filterOpts, lookback)
	if err != nil {
		return nil, err
	}

	q = q.Where("prow_jobs.release = ?", release)
	q = q.Where("prow_job_runs.prow_job_release = ?", release)
	q = q.Where(`prow_job_runs."timestamp" < ?`, reportEnd)
	q = q.Where(`prow_job_runs."timestamp" >= ?`, lookback)

	var rowCount int64
	if err := q.Count(&rowCount).Error; err != nil {
		return nil, err
	}

	if needs.prJoinForSort && !needs.prJoinForFilter {
		q = addPRJoin(q)
	}

	q = q.Order("prow_job_runs.id DESC")

	if pagination == nil {
		pagination = &apitype.Pagination{
			PerPage: int(rowCount),
			Page:    0,
		}
	} else {
		q = q.Limit(pagination.PerPage).Offset(pagination.Page * pagination.PerPage)
	}

	if err := q.Scan(&jobsResult).Error; err != nil {
		return nil, err
	}

	// Phase 2: enrich paginated results in parallel. Each enrichment
	// function writes to disjoint fields of jobsResult, so no locking
	// is needed.
	if len(jobsResult) > 0 {
		ids := make([]int, len(jobsResult))
		for i, jr := range jobsResult {
			ids[i] = jr.ID
		}

		var g errgroup.Group

		g.Go(func() error {
			return enrichJobRunsWithTestNames(dbc, jobsResult, ids, release, lookback)
		})

		if !needs.needsPRJoin() {
			g.Go(func() error {
				return enrichJobRunsWithPRData(dbc, jobsResult, ids, release)
			})
		}

		g.Go(func() error {
			return enrichJobRunsWithAnnotations(dbc, jobsResult, ids, release)
		})

		if err := g.Wait(); err != nil {
			return nil, err
		}
	}

	return &apitype.PaginationResult{
		Rows:      jobsResult,
		TotalRows: rowCount,
		PageSize:  pagination.PerPage,
		Page:      pagination.Page,
	}, nil
}

const jobRunsBaseSelect = `prow_job_runs.id,
	prow_jobs.release,
	prow_jobs.name,
	prow_jobs.name AS job,
	prow_jobs.variants,
	regexp_replace(prow_jobs.name, 'periodic-ci-openshift-(multiarch|release)-(master|main)-(ci|nightly)-[0-9]+.[0-9]+-', '') AS brief_name,
	prow_job_runs.overall_result,
	prow_job_runs.url AS test_grid_url,
	prow_job_runs.url,
	prow_job_runs.succeeded,
	prow_job_runs.infrastructure_failure,
	prow_job_runs.known_failure,
	prow_job_runs."timestamp",
	prow_job_runs.id AS prow_id,
	prow_job_runs.cluster,
	prow_job_runs.labels,
	prow_job_runs.test_failures,
	prow_job_runs.test_flakes`

type jobRunFilterNeeds struct {
	prJoinForSort   bool
	prJoinForFilter bool
}

func (n jobRunFilterNeeds) needsPRJoin() bool { return n.prJoinForSort || n.prJoinForFilter }

func analyzeJobRunFilters(filterOpts *filter.FilterOptions, prColumns sets.Set[string]) jobRunFilterNeeds {
	needs := jobRunFilterNeeds{
		prJoinForSort: prColumns.Has(filterOpts.SortField),
	}
	if filterOpts.Filter == nil {
		return needs
	}
	for _, item := range filterOpts.Filter.Items {
		if prColumns.Has(item.Field) {
			needs.prJoinForFilter = true
		}
	}
	return needs
}

// columnAliases maps filter field names that are SELECT aliases (not base
// table columns) to their table-qualified column expressions. PostgreSQL
// WHERE clauses cannot reference SELECT aliases, so these fields must be
// rewritten before they reach the generic filter system.
var columnAliases = map[string]string{
	"id":                  "prow_job_runs.id",
	"job":                 "prow_jobs.name",
	"brief_name":          "regexp_replace(prow_jobs.name, 'periodic-ci-openshift-(multiarch|release)-(master|main)-(ci|nightly)-[0-9]+.[0-9]+-', '')",
	"prow_id":             "prow_job_runs.id",
	"test_grid_url":       "prow_job_runs.url",
	"timestamp":           `prow_job_runs."timestamp"`,
	"pull_request_link":   "pp.link",
	"pull_request_sha":    "pp.sha",
	"pull_request_org":    "pp.org",
	"pull_request_repo":   "pp.repo",
	"pull_request_author": "pp.author",
}

// testNameStatuses maps test name filter fields to the prow_job_run_tests
// status code used in EXISTS subqueries. ran_test_names uses 0 to mean
// "no status constraint" (matches any test outcome). This is safe because
// TestStatusAbsent (0) is never stored in prow_job_run_tests rows.
var testNameStatuses = map[string]int{
	"ran_test_names":    0,
	"failed_test_names": int(sippyprocessingv1.TestStatusFailure),
	"flaked_test_names": int(sippyprocessingv1.TestStatusFlake),
}

func isTestNameField(field string) bool {
	_, ok := testNameStatuses[field]
	return ok
}

// applyJobRunFilters processes all filter items in a single pass, dispatching
// each to the appropriate handler based on field name. Fields that need
// special SQL (test name EXISTS or table-qualified column aliases) are handled
// directly; the rest use the generic filter SQL generator. All clauses are
// collected and combined with AND or OR based on linkOperator.
func applyJobRunFilters(q *gorm.DB, filterOpts *filter.FilterOptions, lookback time.Time) (*gorm.DB, error) {
	if filterOpts.Filter == nil || len(filterOpts.Filter.Items) == 0 {
		return filter.FilterableDBResult(q, filterOpts, apitype.JobRun{})
	}

	var clauses []string
	var allArgs []any
	for _, item := range filterOpts.Filter.Items {
		var sql string
		var param any
		switch {
		case isTestNameField(item.Field):
			sqlFrag, args, err := testNameFilterSQL(item, lookback)
			if err != nil {
				return nil, &ValidationError{Message: err.Error()}
			}
			clauses = append(clauses, sqlFrag)
			allArgs = append(allArgs, args...)
			continue
		default:
			if col, ok := columnAliases[item.Field]; ok {
				var err error
				sql, param, err = item.FilterItemToSQL(col)
				if err != nil {
					return nil, &ValidationError{Message: err.Error()}
				}
			} else {
				sql, param = item.FilterFieldToSQL(apitype.JobRun{})
			}
		}
		if sql != "" {
			clauses = append(clauses, sql)
			if param != nil {
				allArgs = append(allArgs, param)
			}
		}
	}
	if len(clauses) > 0 {
		joiner := " AND "
		if filterOpts.Filter.LinkOperator == filter.LinkOperatorOr {
			joiner = " OR "
		}
		q = q.Where("("+strings.Join(clauses, joiner)+")", allArgs...)
	}

	sortOpts := *filterOpts
	sortOpts.Filter = &filter.Filter{}
	return filter.FilterableDBResult(q, &sortOpts, apitype.JobRun{})
}

func testNameFilterSQL(item filter.FilterItem, lookback time.Time) (string, []any, error) {
	statusClause := ""
	if status := testNameStatuses[item.Field]; status != 0 {
		statusClause = fmt.Sprintf(" AND prow_job_run_tests.status = %d", status)
	}

	existsBase := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM prow_job_run_tests JOIN tests ON tests.id = prow_job_run_tests.test_id WHERE prow_job_run_tests.prow_job_run_id = prow_job_runs.id AND prow_job_run_tests.prow_job_run_release = prow_jobs.release AND prow_job_run_tests.prow_job_run_timestamp >= ?%s",
		statusClause,
	)

	var sql string
	var args []any
	switch item.Operator {
	case filter.OperatorIsEmpty:
		sql = "NOT " + existsBase + ")"
		args = []any{lookback}
	case filter.OperatorIsNotEmpty:
		sql = existsBase + ")"
		args = []any{lookback}
	case filter.OperatorHasEntry, filter.OperatorEquals:
		sql = existsBase + " AND tests.name = ?)"
		args = []any{lookback, item.Value}
	case filter.OperatorStartsWith:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = []any{lookback, fmt.Sprintf("%s%%", filter.EscapeLikeMetachars(item.Value))}
	case filter.OperatorEndsWith:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = []any{lookback, fmt.Sprintf("%%%s", filter.EscapeLikeMetachars(item.Value))}
	case filter.OperatorContains, filter.OperatorHasEntryContaining:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = []any{lookback, fmt.Sprintf("%%%s%%", filter.EscapeLikeMetachars(item.Value))}
	default:
		return "", nil, fmt.Errorf("unsupported operator %q for field %q", item.Operator, item.Field)
	}
	return filter.WrapNot(sql, item.Not), args, nil
}

func enrichJobRunsWithTestNames(dbc *db.DB, results []apitype.JobRun, ids []int, release string, lookback time.Time) error {
	type testNameResult struct {
		ProwJobRunID    int            `gorm:"column:prow_job_run_id"`
		FailedTestNames pq.StringArray `gorm:"column:failed_test_names;type:text[]"`
		FlakedTestNames pq.StringArray `gorm:"column:flaked_test_names;type:text[]"`
	}
	var nameResults []testNameResult
	nameSQL := fmt.Sprintf(`SELECT pjrt.prow_job_run_id,
			array_agg(t.name) FILTER (WHERE pjrt.status = %d) AS failed_test_names,
			array_agg(t.name) FILTER (WHERE pjrt.status = %d) AS flaked_test_names
		FROM prow_job_run_tests pjrt
			JOIN tests t ON t.id = pjrt.test_id
		WHERE pjrt.prow_job_run_id IN ?
			AND pjrt.status IN (%d, %d)
			AND pjrt.prow_job_run_timestamp >= ?`,
		sippyprocessingv1.TestStatusFailure, sippyprocessingv1.TestStatusFlake, sippyprocessingv1.TestStatusFailure, sippyprocessingv1.TestStatusFlake)
	nameArgs := []any{ids, lookback}
	if len(release) > 0 {
		nameSQL += ` AND pjrt.prow_job_run_release = ?`
		nameArgs = append(nameArgs, release)
	}
	nameSQL += ` GROUP BY pjrt.prow_job_run_id`
	if err := dbc.DB.Raw(nameSQL, nameArgs...).Scan(&nameResults).Error; err != nil {
		return err
	}
	nameMap := make(map[int]*testNameResult, len(nameResults))
	for i := range nameResults {
		nameMap[nameResults[i].ProwJobRunID] = &nameResults[i]
	}
	for i := range results {
		if names, ok := nameMap[results[i].ID]; ok {
			results[i].FailedTestNames = names.FailedTestNames
			results[i].FlakedTestNames = names.FlakedTestNames
		}
	}
	return nil
}

func enrichJobRunsWithPRData(dbc *db.DB, results []apitype.JobRun, ids []int, release string) error {
	type prResult struct {
		ID                int    `gorm:"column:id"`
		PullRequestLink   string `gorm:"column:pull_request_link"`
		PullRequestSHA    string `gorm:"column:pull_request_sha"`
		PullRequestOrg    string `gorm:"column:pull_request_org"`
		PullRequestRepo   string `gorm:"column:pull_request_repo"`
		PullRequestAuthor string `gorm:"column:pull_request_author"`
	}
	var prResults []prResult
	q := dbc.DB.Table("prow_job_run_prow_pull_requests jrpp").
		Select(`DISTINCT ON(jrpp.prow_job_run_id)
			jrpp.prow_job_run_id AS id,
			pp.link AS pull_request_link,
			pp.sha AS pull_request_sha,
			pp.org AS pull_request_org,
			pp.author AS pull_request_author,
			pp.repo AS pull_request_repo`).
		Joins("INNER JOIN prow_pull_requests pp ON pp.id = jrpp.prow_pull_request_id").
		Where("jrpp.prow_job_run_id IN ?", ids).
		Order("jrpp.prow_job_run_id, jrpp.prow_pull_request_id DESC")
	if len(release) > 0 {
		q = q.Where("jrpp.prow_job_run_release = ?", release)
	}
	if err := q.Scan(&prResults).Error; err != nil {
		return err
	}
	prMap := make(map[int]*prResult, len(prResults))
	for i := range prResults {
		prMap[prResults[i].ID] = &prResults[i]
	}
	for i := range results {
		pr, ok := prMap[results[i].ID]
		if !ok {
			continue
		}
		results[i].PullRequestLink = pr.PullRequestLink
		results[i].PullRequestSHA = pr.PullRequestSHA
		results[i].PullRequestOrg = pr.PullRequestOrg
		results[i].PullRequestRepo = pr.PullRequestRepo
		results[i].PullRequestAuthor = pr.PullRequestAuthor
	}
	return nil
}

func enrichJobRunsWithAnnotations(dbc *db.DB, results []apitype.JobRun, ids []int, release string) error {
	var annotations []models.ProwJobRunAnnotation
	annotationQuery := dbc.DB.Where("prow_job_run_id IN ?", ids)
	if len(release) > 0 {
		annotationQuery = annotationQuery.Where("prow_job_run_release = ?", release)
	}
	if err := annotationQuery.Find(&annotations).Error; err != nil {
		return err
	}
	annotationsByRun := make(map[int]apitype.AnnotationMap)
	for _, a := range annotations {
		runID := int(a.ProwJobRunID) //nolint:gosec // DB IDs are well within int range
		if annotationsByRun[runID] == nil {
			annotationsByRun[runID] = make(apitype.AnnotationMap)
		}
		annotationsByRun[runID][a.Key] = a.Value
	}
	for i := range results {
		if am, ok := annotationsByRun[results[i].ID]; ok {
			results[i].Annotations = am
		}
	}
	return nil
}

// FetchJobRun returns a single job run loaded from postgres and populated with the ProwJob and test results.
// If onlyNewTests is true, only new tests are loaded: those not registered in test_ownerships and not
// previously seen in a merged pull request. Otherwise any failed tests are loaded.
func FetchJobRun(dbc *db.DB, jobRunID int64, onlyNewTests bool, preloads []string, logger *log.Entry) (*models.ProwJobRun, error) {
	jobRun := &models.ProwJobRun{}

	// Load the ProwJobRun, ProwJob, and (failed|unknown) tests:
	// TODO: we may want to expand to analyzing flakes here in the future
	q := dbc.DB.Joins("ProwJob").
		Preload("PullRequests")

	// Tests are loaded separately with partition pruning keys, so
	// split any Tests-related preloads from the main query.
	otherPreloads, testPreloads := splitTestPreloads(preloads)
	for _, preload := range otherPreloads {
		q = q.Preload(preload)
	}

	partKeys, err := query.LookupProwJobRunPartitionKeys(dbc.DB, jobRunID)
	if err != nil {
		return nil, fmt.Errorf("looking up partition keys for job run %d: %w", jobRunID, err)
	}

	res := q.Where("prow_job_release = ? AND timestamp = ?", partKeys.ProwJobRelease, partKeys.Timestamp).
		Take(jobRun, jobRunID)
	if res.Error != nil {
		return nil, res.Error
	}

	var tests []models.ProwJobRunTest
	testQuery := dbc.DB.
		Where("prow_job_run_id = ? AND prow_job_run_release = ? AND prow_job_run_timestamp = ?",
			jobRun.ID, jobRun.ProwJobRelease, jobRun.Timestamp)

	if onlyNewTests {
		// A test is "new" if it is not registered in test_ownerships and has
		// not appeared in any job run associated with a merged pull request.
		testQuery = testQuery.
			Where("NOT EXISTS (SELECT 1 FROM test_ownerships tow WHERE tow.test_id = prow_job_run_tests.test_id)").
			Where(`NOT EXISTS (
				SELECT 1 FROM prow_job_run_tests t2
				INNER JOIN prow_job_run_prow_pull_requests prmap ON prmap.prow_job_run_id = t2.prow_job_run_id AND prmap.prow_job_run_release = t2.prow_job_run_release
				INNER JOIN prow_pull_requests prs ON prs.id = prmap.prow_pull_request_id
				WHERE t2.test_id = prow_job_run_tests.test_id
				  AND t2.prow_job_run_release = prow_job_run_tests.prow_job_run_release
				  AND prs.merged_at IS NOT NULL)`)
	} else {
		testQuery = testQuery.Where("status = ?", sippyprocessingv1.TestStatusFailure)
	}

	testQuery = testQuery.Preload("Test").Preload("Suite")
	for _, preload := range testPreloads {
		testQuery = testQuery.Preload(preload)
	}
	if err := testQuery.Find(&tests).Error; err != nil {
		return nil, err
	}
	jobRun.Tests = tests

	jobRunTestCount, err := query.JobRunTestCount(dbc, jobRunID, jobRun.ProwJobRelease, jobRun.Timestamp)
	if err != nil { // should be unusual
		logger.WithError(err).Errorf("Error getting test count for job run %d", jobRunID)
		jobRunTestCount = -1
	}
	jobRun.TestCount = jobRunTestCount

	return jobRun, nil
}

// splitTestPreloads separates Tests-related preloads (e.g. "Tests.ProwJobRunTestOutput")
// from other preloads. Tests-related preloads are returned with the "Tests." prefix stripped
// so they can be applied to the manual test query.
func splitTestPreloads(preloads []string) (other, testRelated []string) {
	for _, p := range preloads {
		if strings.HasPrefix(p, "Tests.") {
			testRelated = append(testRelated, strings.TrimPrefix(p, "Tests."))
		} else {
			other = append(other, p)
		}
	}
	return
}

// findReleaseMatchJobNames looks for the first matches with a common root job name specific to the
// compareRelease and the prowJob variants, starting with the full name.  When no match is found it will iterate while
// removing the leading 'string-'
// and try to find a match until successful or no matches are found.
//
// The use case is for pull request jobs that we want to find a matching periodic that is running the
// same root job.  We use the periodic as the 'standard' to compare test rates.
// e.g.
// pull-ci-openshift-origin-master- e2e-vsphere-ovn-etcd-scaling
// periodic-ci-openshift-release-master-nightly-4.14- e2e-vsphere-ovn-etcd-scaling
// our common root is e2e-vsphere-ovn-etcd-scaling and our compareRelease is 4.14
// if we don't have enough data from the current compareRelease we fall back to include the previous release as well
func findReleaseMatchJobNames(dbc *db.DB, jobRun *models.ProwJobRun, compareRelease string, logger *log.Entry) ([]string, int, error) {
	segments := strings.Split(jobRun.ProwJob.Name, "-")
	logger = logger.WithField("func", "findReleaseMatchJobNames").WithField("job", jobRun.ProwJob.Name)

	// if we don't find enough jobs to match against we can try the prior release
	// and see if it has enough, think about cutover to a new release, etc.

	for i := 0; i < len(segments); i++ {

		// pull-ci-openshift-origin-master-e2e-vsphere-ovn-etcd-scaling
		// ci-openshift-origin-master-e2e-vsphere-ovn-etcd-scaling
		// openshift-origin-master-e2e-vsphere-ovn-etcd-scaling
		// origin-master-e2e-vsphere-ovn-etcd-scaling
		// master-e2e-vsphere-ovn-etcd-scaling
		// e2e-vsphere-ovn-etcd-scaling
		// matches periodic-ci-openshift-release-master-nightly-4.14-e2e-vsphere-ovn-etcd-scaling
		// when we specify the 4.14 release
		name := joinSegments(segments, i, "-")

		if len(name) > 0 {
			jobs, err := query.ProwJobSimilarName(dbc, name, compareRelease)

			if err != nil {
				logger.WithError(err).Errorf("Failed to find similar name for release: %s, root: %s", compareRelease, name)
			}

			if len(jobs) > 0 {
				logger.Debugf("Found %d potential name matches", len(jobs))

				// the first hit we get
				// compare the variants
				// for the matches
				// query the run count for each id
				// and total it up

				allJobNames := make([]string, 0)
				totalJobRunsCount := 0
				hasNeverStableJob := false
				variants := jobRun.ProwJob.Variants
				gosort.Strings(variants)
				for _, job := range jobs {
					// this is a weird way to get the variant we want, but it allows re-use
					// of the existing code.
					// how do we handle never-stable
					if len(job.Variants) == 1 && job.Variants[0] == testidentification.NeverStable {
						hasNeverStableJob = true
					}

					gosort.Strings(job.Variants)
					if stringSlicesEqual(variants, job.Variants) {

						runCount, err := query.ProwJobRunCount(dbc, job.ID, compareRelease, time.Now().Add(-14*24*time.Hour))
						if err != nil {
							logger.WithError(err).Errorf("Failed to query job run count for %d", job.ID)
							continue
						}
						totalJobRunsCount += runCount
						allJobNames = append(allJobNames, job.Name)
					}
				}

				// logging at info for now so we can monitor, can dial down to debug if / when preferred
				if len(allJobNames) > 0 {
					logger.Infof("Matched job name: %s to %v", jobRun.ProwJob.Name, allJobNames)
				}

				var err error
				if hasNeverStableJob {
					err = errors.New(testidentification.NeverStable)
				}

				return allJobNames, totalJobRunsCount, err
			}

		}
	}
	return nil, 0, nil
}

func joinSegments(segments []string, start int, separator string) string {
	if start > len(segments)-1 {
		return ""
	}
	return strings.Join(segments[start:], separator)
}

// JobRunRiskAnalysis checks the test failures and linked bugs for a job run, and reports back an estimated
// risk level for each failed test, and the job run overall.
func JobRunRiskAnalysis(
	ctx context.Context, logger *log.Entry,
	dbc *db.DB, bqc *bigquery.Client, cacheClient cache.Cache,
	jobRun *models.ProwJobRun,
	compareOtherPRs bool,
) (apitype.ProwJobRunRiskAnalysis, error) {
	logger = logger.WithField("func", "JobRunRiskAnalysis")
	// If this job is a Presubmit, compare to test results from master, not presubmits, which may perform
	// worse due to dev code that hasn't merged. We do not presently track presubmits on branches other than
	// master, so it should be safe to assume the latest compareRelease in the db.
	compareRelease := jobRun.ProwJob.Release
	neverStableJob := false
	if compareRelease == models.ReleasePresubmits {
		// TODO: Non-OCP are not supported yet. At least ensure adding new releases doesn't break OCP.
		var err error
		compareRelease, err = query.CurrentActiveRelease(dbc)
		if err != nil {
			return apitype.ProwJobRunRiskAnalysis{}, fmt.Errorf("getting release for presubmit risk analysis: %w", err)
		}
	}

	historicalCount, err := query.ProwJobHistoricalTestCounts(dbc, jobRun.ProwJob.ID, jobRun.ProwJob.Release)

	// if we had an error we will continue the risk analysis and not elevate based on test counts
	if err != nil {
		logger.WithError(err).Error("Error comparing historical job run test count")
		historicalCount = 0
	}

	// -1 indicates an error getting the jobRunTest count we will log an error and skip this validation
	if jobRun.TestCount < 0 {
		logger.Error("Unable to determine job run test count, initializing to historical count")
		jobRun.TestCount = historicalCount
	} else if jobRun.TestCount == 0 {
		// hack since we don't currently get the jobRunTestCount for 4.12 jobs.
		// If the jobRunTestCount is 0 and we are pre 4.13 set the jobRunTestCount to the historicalCount
		preSupportVersion, _ := version.NewVersion("4.12")
		currentVersion, err := version.NewVersion(compareRelease)
		if err != nil {
			logger.WithError(err).Errorf("Failed to parse release '%s' for prow job %d", compareRelease, jobRun.ProwJob.ID)
		} else if preSupportVersion.GreaterThanOrEqual(currentVersion) {
			jobRun.TestCount = historicalCount
		}
	}

	// we want to get a list of job names and a count of jobRunIds and fall back to include prior release if needed,
	// variants don't cover all of our cases, like etcd-scaling so we want to
	// find a job match against releases and analyze the pass rates
	jobNames, totalJobRuns, err := findReleaseMatchJobNames(dbc, jobRun, compareRelease, logger)

	if err != nil {
		if err.Error() == "never-stable" {
			neverStableJob = true
		} else {
			logger.WithError(err).Errorf("Failed to find matching jobIds for: %s", jobRun.ProwJob.Name)
		}
	}

	if totalJobRuns < 20 {
		releases, err := GetReleasesFromDB(ctx, dbc)
		if err != nil {
			logger.WithError(err).Error("Failed to get releases for prior release lookup")
		} else {
			var priorRelease string
			for _, r := range releases {
				if r.Release == compareRelease && r.PreviousRelease != "" {
					priorRelease = r.PreviousRelease
					break
				}
			}
			if priorRelease != "" {
				priorJobNames, _, err := findReleaseMatchJobNames(dbc, jobRun, priorRelease, logger)
				if err != nil {
					// since this is for the prior release we won't return the never-stable error in this case
					if err.Error() != "never-stable" {
						logger.WithError(err).Errorf("Failed to find matching jobIds for: %s", jobRun.ProwJob.Name)
					}
				}
				jobNames = append(jobNames, priorJobNames...)
			}
		}
	}

	logger.Infof("Found %d matching job(s) for: %s", len(jobNames), jobRun.ProwJob.Name)

	// NOTE: we are including bugs for all releases, may want to filter here in future to just those
	// with an AffectsVersions that seems to match our compareRelease?
	jobBugs, err := query.LoadBugsForJobs(dbc, []int{int(jobRun.ProwJob.ID)}, true) // nolint:gosec
	if err != nil {
		logger.WithError(err).Errorf("Error evaluating bugs for prow job: %d", jobRun.ProwJob.ID)
	} else {
		jobRun.ProwJob.Bugs = jobBugs
	}

	// Pre-load test bugs as well:
	if len(jobRun.Tests) <= maxFailuresToFullyAnalyze {
		for i, tr := range jobRun.Tests {
			bugs, err := query.LoadBugsForTest(dbc, tr.Test.Name, true)
			if err != nil {
				logger.WithError(err).Errorf("Error evaluating bugs for prow job: %d, test name: %s", jobRun.ProwJob.ID, tr.Test.Name)
			} else {
				logger.Debugf("Found %d bugs for test '%s'", len(bugs), tr.Test.Name)
				tr.Test.Bugs = bugs
				jobRun.Tests[i] = tr
			}
		}
	}

	return runJobRunAnalysis(ctx,
		bqc, jobRun, compareRelease, historicalCount, neverStableJob, jobNames, logger,
		jobNamesTestResultFunc(dbc, compareRelease),
		variantsTestResultFunc(ctx, dbc, cacheClient),
		compareOtherPRs,
	)
}

// testResultsByJobNameFunc is used for injecting db responses in unit tests.
type testResultsByJobNameFunc func(testName string, jobNames []string) (*apitype.Test, error)

type testResultsByVariantsFunc func(testName string, release, suite string, variants []string, jobNames []string) (*apitype.Test, error)

// jobNamesTestResultFunc looks to match job runs based on the jobnames
func jobNamesTestResultFunc(dbc *db.DB, release string) testResultsByJobNameFunc {
	return func(testName string, jobNames []string) (*apitype.Test, error) {
		if len(jobNames) == 0 {
			return nil, nil
		}

		analyzeSince := time.Now().Add(-14 * 24 * time.Hour)

		q := dbc.DB.Raw(query.QueryTestAnalysis, analyzeSince, release, release, testName, jobNames)
		if q.Error != nil {
			return nil, q.Error
		}

		testReport := apitype.Test{}
		if err := q.First(&testReport).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, err
		}
		testReport.Name = testName
		return &testReport, nil
	}
}

// variantsTestResultFunc looks to match job runs based on variant matches
func variantsTestResultFunc(ctx context.Context, dbc *db.DB, cacheClient cache.Cache) testResultsByVariantsFunc {
	return func(testName, release, suite string, variants []string, jobNames []string) (*apitype.Test, error) {

		fil := &filter.Filter{
			Items: []filter.FilterItem{
				{
					Field:    "name",
					Not:      false,
					Operator: filter.OperatorEquals,
					Value:    testName,
				},
			},
			LinkOperator: "and",
		}
		spec := TestResultsSpec{
			Release:        release,
			Period:         "default",
			Collapse:       false,
			IncludeOverall: true,
			Filter:         fil,
		}
		result, err := spec.buildTestsResultsFromPostgres(ctx, dbc, cacheClient)
		if err != nil {
			return nil, err
		}
		if result.Test != nil {
			result.Test.Variants = append(result.Test.Variants, "Overall")
		}
		gosort.Strings(variants)
		for _, testResult := range result.TestsAPIResult {
			// this is a weird way to get the variant we want, but it allows re-use
			// of the existing code.
			gosort.Strings(testResult.Variants)
			if stringSlicesEqual(variants, testResult.Variants) && testResult.SuiteName == suite {
				if result.Test.CurrentPassPercentage < testResult.CurrentPassPercentage {
					return result.Test, nil
				}
				return &testResult, nil
			}
		}

		// otherwise, what is our best match...
		// do something more expensive and check to see
		// which testResult contains all the variants we have currently
		for _, testResult := range result.TestsAPIResult {
			// we didn't find an exact variant match
			// next best guess is the first variant list that contains all of our known variants
			if stringSubSlicesEqual(variants, testResult.Variants) && testResult.SuiteName == suite {
				if result.Test.CurrentPassPercentage < testResult.CurrentPassPercentage {
					return result.Test, nil
				}
				return &testResult, nil
			}
		}

		return nil, nil
	}
}

func runJobRunAnalysis(ctx context.Context, bqc *bigquery.Client, jobRun *models.ProwJobRun, compareRelease string, historicalRunTestCount int, neverStableJob bool, jobNames []string, logger *log.Entry, testResultsJobNameFunc testResultsByJobNameFunc, testResultsVariantsFunc testResultsByVariantsFunc, compareOtherPRs bool) (apitype.ProwJobRunRiskAnalysis, error) {

	logger = logger.WithField("func", "runJobRunAnalysis").WithField("job", jobRun.ProwJob.Name)
	logger.Infof("analyzing prow job run with %d failed test(s)", len(jobRun.Tests))

	response := apitype.ProwJobRunRiskAnalysis{
		ProwJobRunID:   jobRun.ID,
		ProwJobName:    jobRun.ProwJob.Name,
		Release:        jobRun.ProwJob.Release,
		CompareRelease: compareRelease,
		Tests:          []apitype.TestRiskAnalysis{},
		OverallRisk: apitype.JobFailureRisk{
			Level:                  apitype.FailureRiskLevelNone,
			Reasons:                []string{},
			JobRunTestCount:        jobRun.TestCount,
			JobRunTestFailures:     len(jobRun.Tests),
			NeverStableJob:         neverStableJob,
			HistoricalRunTestCount: historicalRunTestCount,
		},
		OpenBugs: jobRun.ProwJob.Bugs,
	}

	switch {

	// Return early if we see a large gap in the number of tests:
	// order matters, if we have 0 tests that ran && 0 tests that failed we
	// want to compare that here before the 'no test failures' case
	case jobRun.TestCount < (int(float64(historicalRunTestCount) * .75)):
		response.OverallRisk.Level = apitype.FailureRiskLevelIncompleteTests
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			fmt.Sprintf("Tests for this run (%d) are below the historical average (%d): IncompleteTests (not enough tests ran to make a reasonable risk analysis; this could be due to infra, installation, or upgrade problems)", jobRun.TestCount, historicalRunTestCount))
		return response, nil

	// Return early if no tests failed in this run:
	case len(jobRun.Tests) == 0:
		response.OverallRisk.Level = apitype.FailureRiskLevelNone
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			"No test failures found in this job run.")
		return response, nil

	// Return early if we see mass test failures:
	case len(jobRun.Tests) > maxFailuresToFullyAnalyze:
		response.OverallRisk.Level = apitype.FailureRiskLevelHigh
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			fmt.Sprintf("%d tests failed in this run: High", len(jobRun.Tests)))
		return response, nil
	}

	maxTestRisk := apitype.FailureRiskLevelNone

	for _, ft := range jobRun.Tests {

		if ft.Test.Name == testidentification.OpenShiftTestsName || testidentification.IsIgnoredTest(ft.Test.Name) {
			continue
		}

		loggerFields := logger.WithField("test", ft.Test.Name)
		analysis, err := runTestRunAnalysis(ctx, bqc, ft, jobRun, compareRelease, loggerFields, testResultsJobNameFunc, jobNames, testResultsVariantsFunc, neverStableJob, compareOtherPRs)
		if err != nil {
			continue // ignore runs where analysis failed
		}
		if analysis.Risk.Level.Level > maxTestRisk.Level {
			maxTestRisk = analysis.Risk.Level
		}
		response.Tests = append(response.Tests, analysis)
	}
	if maxTestRisk.Level >= response.OverallRisk.Level.Level {
		response.OverallRisk.Level = maxTestRisk
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons, fmt.Sprintf("Maximum failed test risk: %s", maxTestRisk.Name))
	}

	return response, nil
}

// For a failed test, query its pass rates by NURPs, find a matching variant combo, and
// see how often we've passed in the last week.
func runTestRunAnalysis(ctx context.Context, bqc *bigquery.Client, failedTest models.ProwJobRunTest, jobRun *models.ProwJobRun, compareRelease string, logger *log.Entry, testResultsJobNameFunc testResultsByJobNameFunc, jobNames []string, testResultsVariantsFunc testResultsByVariantsFunc, neverStableJob, compareOtherPRs bool) (apitype.TestRiskAnalysis, error) {
	logger.Debug("failed test")

	var testResultsJobNames, testResultsVariants *apitype.Test
	var errJobNames, errVariants error

	// set upper and lower bounds for the number of jobNames we look to match against
	if testResultsJobNameFunc != nil {
		if len(jobNames) < 5 && len(jobNames) > 0 {
			testResultsJobNames, errJobNames = testResultsJobNameFunc(failedTest.Test.Name, jobNames)

			if errJobNames == nil && testResultsJobNames != nil {
				if testResultsJobNames.CurrentRuns == 0 {
					// do we need to prepend the suite name to the test?
					testResultsJobNames, errJobNames = testResultsJobNameFunc(fmt.Sprintf("%s.%s", failedTest.Suite.Name, failedTest.Test.Name), jobNames)
				}
			}

		} else {
			logger.Warningf("Skipping job names test analysis due to jobNames length: %d", len(jobNames))
		}
	}

	// if this matched a neverStableJob we don't want to use the variant match as it will include
	// results from stable jobs and potentially skew results.
	// we will rely on the jobname match, if any, for analysis
	if testResultsVariantsFunc != nil && !neverStableJob {
		testResultsVariants, errVariants = testResultsVariantsFunc(failedTest.Test.Name, compareRelease, failedTest.Suite.Name, jobRun.ProwJob.Variants, jobNames)

		if errVariants == nil && (testResultsVariants == nil || testResultsVariants.CurrentRuns == 0) {
			// do we need to prepend the suite name to the test?
			// drop passing the suite name to the func as we are prepending it to the test name
			testResultsVariants, errVariants = testResultsVariantsFunc(fmt.Sprintf("%s.%s", failedTest.Suite.Name, failedTest.Test.Name), compareRelease, "", jobRun.ProwJob.Variants, jobNames)
		}
	}

	if errJobNames != nil && errVariants != nil {
		logger.WithError(errVariants).Error("Failed test results by variants")
		logger.WithError(errJobNames).Error("Failed test results job names")
		return apitype.TestRiskAnalysis{}, errJobNames
	}

	analysis := apitype.TestRiskAnalysis{
		Name:     failedTest.Test.Name,
		TestID:   failedTest.Test.ID,
		OpenBugs: failedTest.Test.Bugs,
	}
	// Watch out for tests that ran in previous period, but not current, no sense comparing to 0 runs:
	if (testResultsVariants != nil && testResultsVariants.CurrentRuns > 0) || (testResultsJobNames != nil && testResultsJobNames.CurrentRuns > 0) {
		// select the 'best' test result
		risk := selectRiskAnalysisResult(testResultsJobNames, testResultsVariants, jobNames, compareRelease)
		if compareOtherPRs && risk.Level.Level >= apitype.FailureRiskLevelHigh.Level && isHighRiskInOtherPRs(ctx, bqc, failedTest, jobRun) {
			// If the same test/job has high risk in other PRs, we override the risk level
			analysis.Risk = apitype.TestFailureRisk{
				Level: apitype.FailureRiskLevelMedium,
				Reasons: []string{
					"Potential external regression detected for High Risk Test analysis",
				},
			}
		} else {
			analysis.Risk = risk
		}
	} else {
		analysis.Risk = apitype.TestFailureRisk{
			Level: apitype.FailureRiskLevelUnknown,
			Reasons: []string{
				fmt.Sprintf("Unable to find matching test results for variants: %v",
					jobRun.ProwJob.Variants),
			},
		}
	}
	return analysis, nil
}

func isHighRiskInOtherPRs(ctx context.Context, bqc *bigquery.Client, failedTest models.ProwJobRunTest, jobRun *models.ProwJobRun) bool {
	if len(jobRun.PullRequests) == 0 {
		return false
	}
	pr := jobRun.PullRequests[0]
	endTime := jobRun.Timestamp.Add(jobRun.Duration)
	if jobRun.Timestamp.IsZero() {
		endTime = time.Now()
	}
	log.Infof("Evaluating if test '%s' is high risk in other PRs for job %s", failedTest.Test.Name, jobRun.ProwJob.Name)
	_, jobSuffix, found := strings.Cut(jobRun.ProwJob.Name, "pull-ci-"+pr.Org+"-"+pr.Repo)
	if !found {
		return false
	}

	queryStr := `
		SELECT COUNT(*)
		FROM ` + "`openshift-ci-data-analysis.ci_data_autodl.risk_analysis_test_results`" + `
		INNER JOIN ` + "`openshift-gce-devel.ci_analysis_us.jobs`" + ` jobs
		  ON JobRunName=jobs.prowjob_build_id
		WHERE PartitionTime BETWEEN TIMESTAMP(@StartTime) AND TIMESTAMP(@EndTime)
		  AND RiskLevel >= 100
		  AND TestName = @TestName
		  AND (org != @Org OR repo != @Repo OR pr_number != @PRNumber)
		  AND prowjob_job_name LIKE @JobPattern`

	q := bqc.Query(ctx, bqlabel.JobRunHighRisk, queryStr)
	q.Parameters = []bqlib.QueryParameter{
		{
			Name:  "StartTime",
			Value: endTime.Add(-12 * time.Hour).Format(time.RFC3339),
		},
		{
			Name:  "EndTime",
			Value: endTime.Add(3 * time.Hour).Format(time.RFC3339),
		},
		{
			Name:  "TestName",
			Value: failedTest.Test.Name,
		},
		{
			Name:  "Org",
			Value: pr.Org,
		},
		{
			Name:  "Repo",
			Value: pr.Repo,
		},
		{
			Name:  "PRNumber",
			Value: fmt.Sprintf("%d", pr.Number),
		},
		{
			Name:  "JobPattern",
			Value: "%" + jobSuffix,
		},
	}

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("Failed querying high risk items from bigquery")
		return false
	}

	var rowCount int64
	for {
		var values []bqlib.Value
		err := it.Next(&values)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing number of high risk items from bigquery")
			return false
		}
		rowCount = values[0].(int64)
		if rowCount > 0 {
			log.Infof("%d High risk item(s) found in other PRs for job %s test '%s'", rowCount, jobRun.ProwJob.Name, failedTest.Test.Name)
			return true
		}
	}

	return false
}

func selectRiskAnalysisResult(testResultsJobNames, testResultsVariants *apitype.Test, jobNames []string, compareRelease string) apitype.TestFailureRisk {

	var variantRisk, jobRisk apitype.TestFailureRisk

	if testResultsJobNames != nil && testResultsJobNames.CurrentRuns > 0 {
		jobRisk = apitype.TestFailureRisk{
			Level: getSeverityLevelForPassRate(testResultsJobNames.CurrentPassPercentage),
			Reasons: []string{
				fmt.Sprintf("This test has passed %.2f%% of %d runs on jobs %v in the last 14 days.",
					testResultsJobNames.CurrentPassPercentage, testResultsJobNames.CurrentRuns, jobNames),
			},
			CurrentRuns:           testResultsJobNames.CurrentRuns,
			CurrentPassPercentage: testResultsJobNames.CurrentPassPercentage,
			CurrentPasses:         testResultsJobNames.CurrentSuccesses,
		}
	}

	if testResultsVariants != nil && testResultsVariants.CurrentRuns > 0 {
		variantRisk = apitype.TestFailureRisk{
			Level: getSeverityLevelForPassRate(testResultsVariants.CurrentPassPercentage),
			Reasons: []string{
				fmt.Sprintf("This test has passed %.2f%% of %d runs on release %s %v in the last week.",
					testResultsVariants.CurrentPassPercentage, testResultsVariants.CurrentRuns, compareRelease, testResultsVariants.Variants),
			},
			CurrentRuns:           testResultsVariants.CurrentRuns,
			CurrentPassPercentage: testResultsVariants.CurrentPassPercentage,
			CurrentPasses:         testResultsVariants.CurrentSuccesses,
		}

	}

	// if both are empty then return Unknown
	if len(jobRisk.Level.Name) == 0 && len(variantRisk.Level.Name) == 0 {
		return apitype.TestFailureRisk{
			Level:   apitype.FailureRiskLevelUnknown,
			Reasons: []string{"Analysis was not performed for this test due to lack of current runs"},
		}
	}

	switch {
	// if one is empty return the other
	case len(jobRisk.Level.Name) == 0:
		return variantRisk
	case len(variantRisk.Level.Name) == 0:
		return jobRisk
	case containsValue(nonDeterministicRiskLevels, jobRisk.Level.Level):
		// if jobnames nondeterministic then return variants
		return variantRisk
	case containsValue(nonDeterministicRiskLevels, variantRisk.Level.Level):
		// if variants nondeterministic then return jobnames
		return jobRisk
	case variantRisk.Level.Level < jobRisk.Level.Level:
		// biased to return the lower risk level
		return variantRisk
	default:
		return jobRisk
	}
}

func containsValue(values []int, value int) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func getSeverityLevelForPassRate(passPercentage float64) apitype.RiskLevel {
	switch {
	case passPercentage >= 98.0:
		return apitype.FailureRiskLevelHigh
	case passPercentage >= 80:
		return apitype.FailureRiskLevelMedium
	case passPercentage < 80:
		return apitype.FailureRiskLevelLow
	}
	return apitype.FailureRiskLevelUnknown
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func stringSubSlicesEqual(a, b []string) bool {
	// we are going to check if b contains all the values in a

	// have to have something to match on
	// and be less than or equal to b
	if len(a) < 1 {
		return false
	}
	if len(a) > len(b) {
		return false
	}
	for _, v := range a {
		found := false
		for _, s := range b {
			if v == s {
				found = true
			}
		}

		if !found {
			return false
		}
	}
	return true
}

// ClusterOperatorStatus represents the status of a cluster operator
type ClusterOperatorStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// JobRunData contains the comprehensive data for a job run including test failures and cluster operator status
type JobRunData struct {
	// Basic job information
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Release   string `json:"release"`
	Cluster   string `json:"cluster"`
	URL       string `json:"url"`
	GCSBucket string `json:"gcsBucket"`

	// Timing information
	StartTime       time.Time     `json:"startTime"`
	Duration        time.Duration `json:"duration"`
	DurationSeconds float64       `json:"durationSeconds"`

	// Status and results
	OverallResult         string `json:"overallResult"`
	Reason                string `json:"reason"`
	Succeeded             bool   `json:"succeeded"`
	Failed                bool   `json:"failed"`
	InfrastructureFailure bool   `json:"infrastructureFailure"`
	KnownFailure          bool   `json:"knownFailure"`

	// Test information
	TestCount        int               `json:"testCount"`
	TestFailureCount int               `json:"testFailureCount"`
	TestFailures     map[string]string `json:"testFailures,omitempty"`

	// Job metadata
	Variants []string `json:"variants,omitempty"`

	// Cluster and infrastructure information
	ClusterOperators []ClusterOperatorStatus `json:"clusterOperators,omitempty"`
}

// GetJobRunSummary extracts and returns the raw job run data
func GetJobRunSummary(ctx context.Context, dbc *db.DB, gcsClient *storage.Client, jobRunID int64) (*JobRunData, error) {
	jLog := log.WithField("JobRunID", jobRunID)
	dbStart := time.Now()
	jLog.Info("Querying DB for job run data")
	jr, err := FetchJobRun(dbc, jobRunID, false, []string{"Tests.ProwJobRunTestOutput"}, jLog)
	if err != nil {
		return nil, err
	}
	jLog.Infof("DB query complete after %+v", time.Since(dbStart))

	failures := extractTestOutputs(jr)

	// Extract data from GCS bucket
	gcsPath, err := prowloader.GetGCSPathForProwJobURL(jLog, jr.URL)
	if err != nil {
		return nil, err
	}
	bkt := gcsClient.Bucket(jr.GCSBucket)
	gcsJr := gcs.NewGCSJobRun(bkt, gcsPath)

	clusterOperators := getUnavailableOrDegradedOperators(gcsJr, jLog)

	jrData := &JobRunData{
		// Basic job information
		ID:        jr.ID,
		Name:      jr.ProwJob.Name,
		Release:   jr.ProwJob.Release,
		Cluster:   jr.Cluster,
		URL:       jr.URL,
		GCSBucket: jr.GCSBucket,

		// Timing information
		StartTime:       jr.Timestamp,
		Duration:        jr.Duration,
		DurationSeconds: jr.Duration.Seconds(),

		// Status and results
		OverallResult:         string(jr.OverallResult),
		Reason:                jr.OverallResult.String(),
		Succeeded:             jr.Succeeded,
		Failed:                jr.Failed,
		InfrastructureFailure: jr.InfrastructureFailure,
		KnownFailure:          jr.KnownFailure,

		// Test information
		TestCount:        jr.TestCount,
		TestFailureCount: jr.TestFailures,
		TestFailures:     failures,

		// Job metadata
		Variants: jr.ProwJob.Variants,

		// Cluster and infrastructure information
		ClusterOperators: clusterOperators,
	}

	return jrData, nil
}

func getUnavailableOrDegradedOperators(jr *gcs.GCSJobRun, jLog *log.Entry) []ClusterOperatorStatus {
	start := time.Now()
	jLog.Info("Fetching cluster operators...")
	// Operator statuses
	coData := jr.FindFirstFile("", regexp.MustCompile("clusteroperators.json"))
	if coData == nil {
		jLog.Infof("Cluster operators not found in %+v", time.Since(start))
		return nil
	}

	var statuses []ClusterOperatorStatus
	var coList openshift.ClusterOperatorList
	if err := json.Unmarshal(coData, &coList); err != nil {
		jLog.WithError(err).Warn("Failed to parse cluster operator list")
		return nil
	}
	for _, co := range coList.Items {
		for _, condition := range co.Status.Conditions {
			if (condition.Type == "Degraded" && condition.Status == "True") || (condition.Type == "Available" && condition.Status == "False") {
				statuses = append(statuses, ClusterOperatorStatus{
					Name:    co.Metadata.Name,
					Status:  condition.Status,
					Reason:  condition.Reason,
					Message: condition.Message,
				})
			}
		}

	}
	jLog.Infof("Cluster operators found in %+v", time.Since(start))
	return statuses
}

func extractTestOutputs(jr *models.ProwJobRun) map[string]string {
	failures := make(map[string]string)
	for _, test := range jr.Tests {
		// skip synthetic tests
		if strings.Contains(test.Test.Name, "sig-sippy") {
			continue
		}

		if sippyprocessingv1.TestStatus(test.Status) == sippyprocessingv1.TestStatusFailure {
			output := test.ProwJobRunTestOutput.Output
			// some tests are very chatty, get the last 256 characters where
			// the meat of the failure probably is.
			if len(output) > 256 {
				output = output[len(output)-256:]
			}
			failures[test.Test.Name] = output
		}
	}
	return failures
}
