package prowloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/util/sets"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db/query"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/testconversion"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/github/commenter"
	"github.com/openshift/sippy/pkg/releaseoverride"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"
)

// gcsPathStrip is used to strip out everything but the path, i.e. match "/view/gs/origin-ci-test/"
// from the path "/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.14-e2e-gcp-sdn/1737420379221135360"
var gcsPathStrip = regexp.MustCompile(`.*/gs/[^/]+/`)

type ProwLoader struct {
	ctx                          context.Context
	dbc                          *db.DB
	errors                       []error
	githubClient                 *github.Client
	bigQueryClient               *bqcachedclient.Client
	maxConcurrency               int
	prowJobCache                 map[string]*models.ProwJob
	variantManager               testidentification.VariantManager
	syntheticTestManager         synthetictests.SyntheticTestManager
	syntheticReleaseJobOverrides *releaseoverride.SyntheticReleaseOverrides
	releases                     []string
	releaseSet                   sets.Set[string]
	releaseRegexps               map[string][]*regexp.Regexp
	config                       *v1config.SippyConfig
	ghCommenter                  *commenter.GitHubCommenter
	gcsClient                    *storage.Client
	promPusher                   *push.Pusher
	loadSince                    *time.Time
	labelsCache                  map[string]pq.StringArray
}

func New(
	ctx context.Context,
	dbc *db.DB,
	gcsClient *storage.Client,
	bigQueryClient *bqcachedclient.Client,
	githubClient *github.Client,
	variantManager testidentification.VariantManager,
	syntheticTestManager synthetictests.SyntheticTestManager,
	releases []string,
	config *v1config.SippyConfig,
	ghCommenter *commenter.GitHubCommenter,
	promPusher *push.Pusher,
	loadSince *time.Time,
	syntheticReleaseJobOverrides *releaseoverride.SyntheticReleaseOverrides) *ProwLoader {

	compiledRegexps := make(map[string][]*regexp.Regexp, len(releases))
	for _, release := range releases {
		if cfg, ok := config.Releases[release]; ok {
			for _, expr := range cfg.Regexp {
				re, err := regexp.Compile(expr)
				if err != nil {
					log.WithError(err).WithField("release", release).WithField("regex", expr).Error("invalid regex in configuration")
					continue
				}
				compiledRegexps[release] = append(compiledRegexps[release], re)
			}
		}
	}

	return &ProwLoader{
		ctx:                          ctx,
		dbc:                          dbc,
		gcsClient:                    gcsClient,
		githubClient:                 githubClient,
		bigQueryClient:               bigQueryClient,
		maxConcurrency:               50,
		syntheticTestManager:         syntheticTestManager,
		syntheticReleaseJobOverrides: syntheticReleaseJobOverrides,
		variantManager:               variantManager,
		releases:                     releases,
		releaseSet:                   sets.New[string](releases...),
		releaseRegexps:               compiledRegexps,
		config:                       config,
		ghCommenter:                  ghCommenter,
		promPusher:                   promPusher,
		loadSince:                    loadSince,
	}
}

const DefaultLookbackDays = 14

func resolveFrom(since *time.Time, to time.Time) time.Time {
	if since != nil {
		return *since
	}
	return to.AddDate(0, 0, -DefaultLookbackDays)
}

func (pl *ProwLoader) resolveLoadSince() time.Time {
	return resolveFrom(pl.loadSince, time.Now())
}

var clusterDataDateTimeName = regexp.MustCompile(`cluster-data_(?P<DATE>.*)-(?P<TIME>.*).json`)

var prowLoaderQueriedMetricGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "sippy_prow_jobs_loaded",
	Help: "The number of jobs loaded (queried)",
})

var prowLoaderProcessedMetricGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "sippy_prow_jobs_processed",
	Help: "The number of jobs processed (new)",
})

type DateTimeName struct {
	Name string
	Date string
	Time string
}

func loadProwJobCache(dbc *db.DB) (map[string]*models.ProwJob, error) {
	prowJobCache := map[string]*models.ProwJob{}
	var allJobs []*models.ProwJob
	if err := dbc.DB.Model(&models.ProwJob{}).Find(&allJobs).Error; err != nil {
		return nil, fmt.Errorf("loading prow job cache: %w", err)
	}
	for _, j := range allJobs {
		prowJobCache[j.Name] = j
	}
	log.Infof("job cache created with %d entries from database", len(prowJobCache))
	return prowJobCache, nil
}

func (pl *ProwLoader) Name() string {
	return "prow"
}

func (pl *ProwLoader) Errors() []error {
	return pl.errors
}

// ensurePartitions creates necessary partitions for partitioned tables.
// It uses the release list from pl.releases and determines the date range based on:
//   - pl.loadSince if available, otherwise looks back one week
//   - Creates partitions 2 days forward from now
func (pl *ProwLoader) ensurePartitions() error {
	startDate := pl.resolveLoadSince()

	// Create partitions 2 days forward from now
	endDate := time.Now().AddDate(0, 0, 2)

	log.Infof("Ensuring partitions for releases %v from %s to %s",
		pl.releases, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// https://github.com/openshift/sippy/blob/main/pkg/dataloader/prowloader/prow.go#L473 bq imports based on modified time which can include job_run_start_time a day earlier
	// add grace to ensure we have a valid partition for new dbs.
	count, err := pl.dbc.EnsurePartitions(pl.releases, startDate.AddDate(0, 0, -1), endDate, false)
	if err != nil {
		return fmt.Errorf("failed to ensure partitions: %w", err)
	}

	log.Infof("Ensured %d partitions across all partitioned tables", count)
	return nil
}

func (pl *ProwLoader) Load() {
	start := time.Now()

	log.Infof("started loading prow jobs to DB...")

	// Update unmerged PR statuses in case any have merged
	if err := pl.syncPRStatus(); err != nil {
		pl.errors = append(pl.errors, errors.Wrap(err, "error in syncPRStatus"))
	}

	// Grab the ProwJob definitions from prow or CI bigquery. Note that these are the Kube
	// ProwJob CRDs, not our sippy db model ProwJob.
	var prowJobs []prow.ProwJob
	// Fetch/update job data
	if pl.bigQueryClient != nil {
		var bqErrs []error
		prowJobs, bqErrs = pl.fetchProwJobsFromOpenShiftBigQuery()
		if len(bqErrs) > 0 {
			pl.errors = append(pl.errors, bqErrs...)
		}
	} else {
		jobsJSON, err := fetchJobsJSON(pl.config.Prow.URL)
		if err != nil {
			pl.errors = append(pl.errors, errors.Wrap(err, "error fetching job JSON data from prow"))
			return
		}
		prowJobs, err = jobsJSONToProwJobs(jobsJSON)
		if err != nil {
			pl.errors = append(pl.errors, errors.Wrap(err, "error decoding job JSON data from prow"))
			return
		}
	}

	// Ensure we have partitions for the new data
	if err := pl.ensurePartitions(); err != nil {
		pl.errors = append(pl.errors, errors.Wrap(err, "failed to ensure partitions"))
		return
	}

	// Clean up old partitions (detach partitions older than 100 days, drop detached partitions older than 110 days)
	if detached, dropped, err := pl.dbc.CleanupPartitions(false); err != nil {
		log.WithError(err).Warning("failed to cleanup old partitions, continuing with load")
		// Don't fail the entire load if partition cleanup fails
	} else {
		log.Infof("Partition cleanup complete: detached %d, dropped %d", detached, dropped)
	}

	// Pre-fetch labels for all jobs in bulk instead of one BQ query per job.
	if lc, err := pl.prefetchLabels(prowJobs); err == nil {
		pl.labelsCache = lc
	} else {
		pl.errors = append(pl.errors, errors.Wrap(err, "error pre-fetching labels from BigQuery"))
	}

	prowLoaderQueriedMetricGauge.Set(float64(len(prowJobs)))

	// Match jobs to releases and bulk-upsert ProwJob definitions before
	// the concurrent processing loop. The prowJobCache is read-only after
	// this point.
	entries, err := pl.preprocessProwJobs(pl.ctx, prowJobs)
	if err != nil {
		pl.errors = append(pl.errors, errors.Wrap(err, "error preprocessing prow jobs"))
		return
	}

	fetchCtx, cancelFetch := context.WithCancel(pl.ctx)
	defer cancelFetch()

	queue := make(chan *prow.ProwJob)
	results := make(chan *jobRunResult, len(entries))
	fetchErrsCh := make(chan error, len(entries))

	go func() {
		defer close(queue)
		for i := range entries {
			select {
			case queue <- entries[i]:
			case <-fetchCtx.Done():
				return
			}
		}
	}()

	var fetchWg sync.WaitGroup
	for i := 0; i < pl.maxConcurrency; i++ {
		fetchWg.Add(1)
		go func(ctx context.Context) {
			defer fetchWg.Done()
			for pj := range queue {
				if err := ctx.Err(); err != nil {
					break
				}
				result, err := pl.fetchJobRunResult(ctx, pj)
				if err != nil {
					fetchErrsCh <- err
					log.WithError(err).WithField("job", pj.Spec.Job).WithField("buildID", pj.Status.BuildID).
						Warning("couldn't fetch job, continuing")
					continue
				}
				if result != nil {
					results <- result
				}
			}
		}(fetchCtx)
	}
	go func() {
		fetchWg.Wait()
		close(results)
		close(fetchErrsCh)
	}()

	pl.accumulateAndWriteJobRuns(pl.ctx, results)

	for err := range fetchErrsCh {
		pl.errors = append(pl.errors, err)
	}

	// load the test analysis by job data into tables partitioned by day, letting bigquery do the
	// heavy lifting for us.
	err = pl.loadDailyTestAnalysisByJob(pl.ctx)
	if err != nil {
		pl.errors = append(pl.errors, errors.Wrap(err, "error updating daily test analysis by job"))
	}

	if len(pl.errors) > 0 {
		log.Warningf("encountered %d errors while importing job runs", len(pl.errors))
	}
	log.Infof("finished importing new job runs in %+v", time.Since(start))

	if pl.promPusher != nil {
		pl.promPusher.Collector(prowLoaderQueriedMetricGauge)
		pl.promPusher.Collector(prowLoaderProcessedMetricGauge)
	}
}

// tempBQTestAnalysisByJobForDate is a dupe type to work around date parsing issues.
type tempBQTestAnalysisByJobForDate struct {
	Date     civil.Date
	TestID   uint
	Release  string
	TestName string `bigquery:"test_name"`
	JobName  string `bigquery:"job_name"`
	Runs     int
	Passes   int
	Flakes   int
	Failures int
}

// getTestAnalysisByJobFromToDates uses the last daily report date to calculate the
// date range we should request from bigquery for import. We don't want to import
// the prior day if jobs are still running, so if we're not at least 8 hours into
// the current day (UTC), we will keep waiting to import yesterday. Once we cross
// that threshold we will import. (assume hourly imports)
// If our most recent import is yesterday, we're done for the day.
// If the lastDailySummary is empty, this implies a new database, and we'll do an initial
// bulk load.
//
// Returns a slice of day strings YYYY-MM-DD in ascending order. We'll import a day at
// a time, with a separate transaction for each. If something goes wrong we can fail and
// pick up at that date the next time.
//
// At present in prod, each day takes about 20 minutes
func getTestAnalysisByJobFromToDates(lastDailySummary, now time.Time, loadSince *time.Time) []string {
	to := now.UTC().Add(-32 * time.Hour)

	// If this is a new db, do an initial larger import:
	if lastDailySummary.IsZero() {
		return DaysBetween(resolveFrom(loadSince, to), to)
	}

	ldsStr := lastDailySummary.UTC().Format("2006-01-02")
	if ldsStr == to.Format("2006-01-02") {
		return []string{}
	}
	from := lastDailySummary.UTC().Add(24 * time.Hour)
	return DaysBetween(from, to)
}

// DaysBetween returns a slice of strings representing each day in YYYY-MM-DD format between two dates
func DaysBetween(start, end time.Time) []string {
	var days []string

	// Normalize times to midnight to count full days
	start = start.Truncate(24 * time.Hour)
	end = end.Truncate(24 * time.Hour)

	// Ensure start is before or equal to end
	if end.Before(start) {
		start, end = end, start
	}

	// Iterate from start to end date
	for d := start; !d.After(end); d = d.Add(24 * time.Hour) {
		days = append(days, d.Format("2006-01-02"))
	}

	return days
}

// NextDay takes a date string in YYYY-MM-DD format and returns the date string for the following day.
func NextDay(dateStr string) (string, error) {
	// Parse the input date string
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date format: %v", err)
	}

	// Add one day to the parsed date
	nextDay := date.Add(24 * time.Hour)

	// Format the next day back to YYYY-MM-DD
	return nextDay.Format("2006-01-02"), nil
}

// loadDailyTestAnalysisByJob loads test analysis data into partitioned tables in postgres, one per
// day. The data is calculated by querying bigquery to do the heavy lifting for us. Each day is committed
// transactionally so the process is safe to interrupt and resume later. The process takes about 20 minutes
// per day at the time of writing, so an initial load for all releases can be quite time consuming.
func (pl *ProwLoader) loadDailyTestAnalysisByJob(ctx context.Context) error {

	// Figure out our last imported daily summary.
	var lastDailySummary time.Time
	row := pl.dbc.DB.Table("test_analysis_by_job_by_dates").Select("MAX(date)").Row()

	// Ignoring error, the function below handles the zero time if needed: (new db)
	_ = row.Scan(&lastDailySummary)

	importDates := getTestAnalysisByJobFromToDates(lastDailySummary, time.Now(), pl.loadSince)
	if len(importDates) == 0 {
		log.Info("test analysis summary already completed today")
		return nil
	}
	log.Infof("importing test analysis by job for dates: %v", importDates)

	jobCache, err := query.LoadProwJobCache(pl.dbc)
	if err != nil {
		log.WithError(err).Error("error loading job cache")
		return err
	}

	testCache, err := query.LoadTestCache(pl.dbc, []string{})
	if err != nil {
		log.WithError(err).Error("error loading test cache")
		return err
	}

	for _, dateToImport := range importDates {
		dLog := log.WithField("date", dateToImport)

		dLog.Infof("Loading test analysis by job daily summaries")

		q := pl.bigQueryClient.Query(ctx, bqlabel.ProwLoaderTestAnalysis, fmt.Sprintf(`WITH
  deduped_testcases AS (
  SELECT
    junit.*,
    ROW_NUMBER() OVER(PARTITION BY file_path, test_name, testsuite ORDER BY CASE WHEN flake_count > 0 THEN 0 WHEN success_val > 0 THEN 1 ELSE 2 END ) AS row_num,
    jobs. prowjob_job_name AS variant_registry_job_name,
    jobs.org,
    jobs.repo,
    jobs.pr_number,
    jobs.pr_sha,
    CASE
      WHEN flake_count > 0 THEN 0
      ELSE success_val
  END
    AS adjusted_success_val,
    CASE
      WHEN flake_count > 0 THEN 1
      ELSE 0
  END
    AS adjusted_flake_count
  FROM
    %s.junit
  INNER JOIN
    %s.jobs jobs
  ON
    junit.prowjob_build_id = jobs.prowjob_build_id
    AND DATE(jobs.prowjob_start) <= DATE(@DateToImport)
  WHERE
    DATE(junit.modified_time) = DATE(@DateToImport)
    AND skipped = FALSE )
SELECT
  test_name,
  DATE(modified_time) AS date,
  prowjob_name AS job_name,
  branch AS release,
  COUNT(*) AS runs,
  SUM(adjusted_success_val) AS passes,
  SUM(adjusted_flake_count) AS flakes,
FROM
  deduped_testcases
WHERE
  row_num = 1
  AND branch IN UNNEST(@Releases)
GROUP BY
  test_name,
  date,
  release,
  prowjob_name
ORDER BY
  date,
  test_name,
  prowjob_name
`, pl.bigQueryClient.Dataset, pl.bigQueryClient.Dataset))
		q.Parameters = []bigquery.QueryParameter{
			{
				Name:  "DateToImport",
				Value: dateToImport,
			},
			{
				Name:  "Releases",
				Value: pl.releases,
			},
		}
		it, err := q.Read(ctx)
		if err != nil {
			dLog.WithError(err).Error("error querying test analysis from bigquery")
			return err
		}

		insertRows := []models.TestAnalysisByJobByDate{}
		for {
			row := tempBQTestAnalysisByJobForDate{}
			err := it.Next(&row)
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.WithError(err).Error("error parsing prowjob from bigquery")
				return err
			}
			psqlDate := pgtype.Date{}
			err = psqlDate.Set(row.Date.String())
			if err != nil {
				return err
			}

			// Skip jobs and tests we don't know about in our postgres db:
			test, ok := testCache[row.TestName]
			if !ok {
				continue
			}

			if _, ok := jobCache[row.JobName]; !ok {
				continue
			}
			// we have to infer failures due to the bigquery query we leveraged:
			failures := row.Runs - row.Passes - row.Flakes

			// convert to a db row for postgres insertion:
			psqlRow := models.TestAnalysisByJobByDate{
				Date:     row.Date.In(time.UTC),
				TestID:   test.ID,
				Release:  row.Release,
				TestName: row.TestName,
				JobName:  row.JobName,
				Runs:     row.Runs,
				Passes:   row.Passes,
				Flakes:   row.Flakes,
				Failures: failures,
			}
			insertRows = append(insertRows, psqlRow)
		}
		st := time.Now()
		dLog.Infof("inserting %d rows", len(insertRows))
		n, err := pl.dbc.CopyFrom(ctx, "test_analysis_by_job_by_dates",
			[]string{"date", "test_id", "release", "job_name", "test_name", "runs", "passes", "flakes", "failures"},
			pgx.CopyFromSlice(len(insertRows), func(i int) ([]any, error) {
				r := &insertRows[i]
				return []any{
					r.Date,
					r.TestID,
					r.Release,
					r.JobName,
					r.TestName,
					r.Runs,
					r.Passes,
					r.Flakes,
					r.Failures,
				}, nil
			}),
		)
		if err != nil {
			return fmt.Errorf("COPY test_analysis_by_job_by_dates: %w", err)
		}
		dLog.Infof("inserted %d rows in %s", n, time.Since(st))
	}
	return nil
}

// matchRelease returns the release a prow job belongs to, or "" if it
// doesn't match any configured release.
func (pl *ProwLoader) matchRelease(jobName string) string {
	if release, ok := pl.syntheticReleaseJobOverrides.Lookup(jobName); ok {
		if pl.releaseSet.Has(release) {
			return release
		}
		return ""
	}

	for _, release := range pl.releases {
		cfg, ok := pl.config.Releases[release]
		if !ok {
			continue
		}
		if val, ok := cfg.Jobs[jobName]; val && ok {
			return release
		}
		for _, re := range pl.releaseRegexps[release] {
			if re.MatchString(jobName) {
				return release
			}
		}
	}
	return ""
}

// preprocessProwJobs matches each BigQuery prow job to a release, filters out
// already-processed runs and non-terminal states, bulk-upserts ProwJob
// definitions, and returns only entries that need GCS fetching.
func (pl *ProwLoader) preprocessProwJobs(ctx context.Context, prowJobs []prow.ProwJob) ([]*prow.ProwJob, error) {
	type candidate struct {
		pj      *prow.ProwJob
		release string
		id      uint64
	}

	var candidates []candidate
	seenJobs := sets.New[string]()
	var jobDefs []models.ProwJob
	var candidateIDs []uint

	for i := range prowJobs {
		pj := &prowJobs[i]

		if pj.Status.State == prow.PendingState || pj.Status.State == prow.TriggeredState {
			continue
		}

		release := pl.matchRelease(pj.Spec.Job)
		if release == "" {
			continue
		}

		id, err := strconv.ParseUint(pj.Status.BuildID, 10, 63)
		if err != nil {
			continue
		}

		candidates = append(candidates, candidate{pj: pj, release: release, id: id})
		candidateIDs = append(candidateIDs, uint(id))

		if seenJobs.Has(pj.Spec.Job) {
			continue
		}
		seenJobs.Insert(pj.Spec.Job)

		jobDefs = append(jobDefs, models.ProwJob{
			Name:        pj.Spec.Job,
			Kind:        models.ProwKind(pj.Spec.Type),
			Release:     release,
			Variants:    pl.variantManager.IdentifyVariants(pj.Spec.Job),
			TestGridURL: pl.generateTestGridURL(release, pj.Spec.Job).String(),
		})
	}

	newIDs, err := pl.findNewJobRunIDs(ctx, candidateIDs)
	if err != nil {
		return nil, fmt.Errorf("finding new job run IDs: %w", err)
	}

	var entries []*prow.ProwJob
	for _, c := range candidates {
		if newIDs.Has(uint(c.id)) {
			entries = append(entries, c.pj)
		}
	}

	log.WithFields(log.Fields{
		"total":      len(prowJobs),
		"candidates": len(candidates),
		"new":        len(entries),
	}).Info("filtered prow jobs for processing")

	log.WithField("jobs", len(jobDefs)).Info("bulk upserting ProwJob definitions")
	const prowJobBatchSize = 100
	for i := 0; i < len(jobDefs); i += prowJobBatchSize {
		batch := jobDefs[i:min(i+prowJobBatchSize, len(jobDefs))]
		if err := pl.dbc.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"kind", "release", "variants", "test_grid_url", "updated_at",
			}),
		}).Create(&batch).Error; err != nil {
			return nil, fmt.Errorf("upserting ProwJob batch: %w", err)
		}
	}

	cache, err := loadProwJobCache(pl.dbc)
	if err != nil {
		return nil, err
	}
	pl.prowJobCache = cache
	return entries, nil
}

func (pl *ProwLoader) findNewJobRunIDs(ctx context.Context, candidateIDs []uint) (sets.Set[uint], error) {
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	sqlDB, err := pl.dbc.DB.DB()
	if err != nil {
		return nil, fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_candidate_ids", candidateIDs,
		[]db.TempColumn[uint]{
			{Name: "id", Type: "bigint NOT NULL", Value: func(id *uint) any { return *id }},
		},
	)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	rows, err := conn.Query(ctx, `
		SELECT t.id FROM tmp_candidate_ids t
		LEFT JOIN prow_job_runs r ON r.id = t.id
		WHERE r.id IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("querying new job run IDs: %w", err)
	}
	var newIDs []uint
	for rows.Next() {
		var id uint
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning new job run ID: %w", err)
		}
		newIDs = append(newIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating new job run IDs: %w", err)
	}

	return sets.New(newIDs...), nil
}

func (pl *ProwLoader) syncPRStatus() error {
	if pl.githubClient == nil {
		log.Infof("No GitHub client, skipping PR sync")
		return nil
	}

	pulls := make([]models.ProwPullRequest, 0)
	if res := pl.dbc.DB.
		Table("prow_pull_requests").
		Where("merged_at IS NULL").Scan(&pulls); res.Error != nil && !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return errors.Wrap(res.Error, "could not fetch prow_pull_requests")
	}

	for _, pr := range pulls {
		logger := log.WithField("org", pr.Org).
			WithField("repo", pr.Repo).
			WithField("number", pr.Number).
			WithField("sha", pr.SHA)

		// first check to see if this pr has recently closed (indicating it may have merged)
		recentMergedAt, mergeCommitSha, err := pl.githubClient.IsPrRecentlyMerged(pr.Org, pr.Repo, pr.Number)

		// the client should have logged the error, we want
		// to see if we are rate limited or not, if so return
		// otherwise keep processing
		if err != nil {
			if pl.githubClient.IsWithinRateLimitThreshold() {
				return err
			}
		}

		if recentMergedAt != nil {
			// we have the recentMergedAt but, we don't know if it is associated with this SHA so do
			// the SHA specific verification
			if mergeCommitSha != nil && *mergeCommitSha == pr.SHA {
				if pr.MergedAt != recentMergedAt {
					pr.MergedAt = recentMergedAt
					if res := pl.dbc.DB.Save(pr); res.Error != nil {
						logger.WithError(res.Error).Errorf("unexpected error updating pull request %s (%s)", pr.Link, pr.SHA)
						continue
					}
				}
			}

			// if we see that any sha has merged for this pr then we should clear out any risk analysis pending comment records
			// if we don't get them here we will catch them before writing the risk analysis comment
			// but, we should clean up here if possible
			pendingComments, err := pl.ghCommenter.QueryPRPendingComments(pr.Org, pr.Repo, pr.Number, models.CommentTypeRiskAnalysis)

			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				logger.WithError(err).Error("Unable to fetch pending comments ")
			}

			for _, pc := range pendingComments {
				pcp := pc
				pl.ghCommenter.ClearPendingRecord(pcp.Org, pcp.Repo, pcp.PullNumber, pcp.SHA, models.CommentTypeRiskAnalysis, &pcp)
			}
		}
	}

	return nil
}

func fetchJobsJSON(prowURL string) ([]byte, error) {
	resp, err := http.Get(prowURL) // #nosec G107
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func jobsJSONToProwJobs(jobJSON []byte) ([]prow.ProwJob, error) {
	results := make(map[string][]prow.ProwJob)
	if err := json.Unmarshal(jobJSON, &results); err != nil {
		return nil, err
	}
	return results["items"], nil
}

func (pl *ProwLoader) generateTestGridURL(release, jobName string) *url.URL {
	if releaseConfig, ok := pl.config.Releases[release]; ok {
		dashboard := "redhat-openshift-ocp-release-" + release
		blockingJobs := sets.New(releaseConfig.BlockingJobs...)
		informingJobs := sets.New(releaseConfig.InformingJobs...)
		jobType := ""
		if blockingJobs.Has(jobName) {
			jobType = "blocking"
		} else if informingJobs.Has(jobName) {
			jobType = "informing"
		}
		if len(jobType) != 0 {
			dashboard = dashboard + "-" + jobType
			return util.URLForJob(dashboard, jobName)
		}
	}
	return &url.URL{}
}

func GetClusterDataBytes(ctx context.Context, bkt *storage.BucketHandle, path string, matches []string) ([]byte, error) {
	// get the variant cluster data for this job run
	gcsJobRun := gcs.NewGCSJobRun(bkt, path)

	// return empty struct to pass along
	match := findMostRecentDateTimeMatch(matches)
	if match == "" {
		return []byte{}, nil
	}

	bytes, err := gcsJobRun.GetContent(ctx, match)
	if err != nil {
		log.WithError(err).Errorf("failed to read cluster-data bytes for: %s", match)
		return []byte{}, err
	} else if bytes == nil {
		log.Warnf("empty cluster-data bytes found for: %s", match)
		return []byte{}, nil
	}

	return bytes, nil
}

func ParseVariantDataFile(bytes []byte) (map[string]string, error) {
	rawJSONMap := make(map[string]interface{})
	err := json.Unmarshal(bytes, &rawJSONMap)
	if err != nil {
		log.WithError(err).Errorf("failed to unmarshal prow cluster data")
		return map[string]string{}, err
	}
	// Convert the raw json map to string->string, discarding anything that doesn't parse to a string.
	clusterData := map[string]string{}
	for k, v := range rawJSONMap {
		if sv, ok := v.(string); ok {
			clusterData[k] = sv
		}
	}
	return clusterData, nil
}

func findMostRecentDateTimeMatch(names []string) string {
	if len(names) < 1 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}

	// get the times stamps and compare
	currMatchDateTime := extractDateTimeName(names[0])
	for _, m := range names[1:] {
		nextMatchDateTime := extractDateTimeName(m)

		if currMatchDateTime == nil {
			currMatchDateTime = nextMatchDateTime
			continue
		}
		if nextMatchDateTime != nil {
			mostRecentMatchDateTime := mostRecentDateTimeName(*currMatchDateTime, *nextMatchDateTime)
			currMatchDateTime = &mostRecentMatchDateTime
		}
	}

	if currMatchDateTime == nil {
		return ""
	}
	return currMatchDateTime.Name
}

func extractDateTimeName(name string) *DateTimeName {
	if !clusterDataDateTimeName.MatchString(name) {
		log.Errorf("Name did not match date time format: %s", name)
		return nil
	}

	dateTimeName := &DateTimeName{Name: name}
	subMatches := clusterDataDateTimeName.FindStringSubmatch(name)
	subNames := clusterDataDateTimeName.SubexpNames()
	for i, sName := range subNames {

		switch sName {
		case "DATE":
			dateTimeName.Date = subMatches[i]
		case "TIME":
			dateTimeName.Time = subMatches[i]
		}
	}

	if len(dateTimeName.Date) > 0 && len(dateTimeName.Time) > 0 {
		return dateTimeName
	}
	return nil
}

func mostRecentDateTimeName(one, two DateTimeName) DateTimeName {
	oneDate, err := strconv.ParseInt(one.Date, 10, 64)
	if err != nil {
		log.WithError(err).Errorf("Error parsing date for %s", one.Name)
	}

	twoDate, err := strconv.ParseInt(two.Date, 10, 64)
	if err != nil {
		log.WithError(err).Errorf("Error parsing date for %s", two.Name)
	}

	if oneDate > twoDate {
		return one
	}

	if twoDate > oneDate {
		return two
	}

	// they are the same so compare the times
	oneTime, err := strconv.ParseInt(one.Time, 10, 64)
	if err != nil {
		log.WithError(err).Errorf("Error parsing time for %s", one.Name)
	}

	twoTime, err := strconv.ParseInt(two.Time, 10, 64)
	if err != nil {
		log.WithError(err).Errorf("Error parsing time for %s", two.Name)
	}

	if oneTime > twoTime {
		return one
	}

	return two
}

func (pl *ProwLoader) fetchJobRunResult(ctx context.Context, pj *prow.ProwJob) (*jobRunResult, error) {
	pjLog := log.WithFields(log.Fields{
		"job":     pj.Spec.Job,
		"buildID": pj.Status.BuildID,
		"start":   pj.Status.StartTime,
	})

	id, err := strconv.ParseUint(pj.Status.BuildID, 10, 63)
	if err != nil {
		pjLog.Warningf("skipping, couldn't parse build ID: %+v", err)
		return nil, nil
	}

	dbProwJob, ok := pl.prowJobCache[pj.Spec.Job]
	if !ok {
		pjLog.Warningf("skipping, ProwJob not found in cache")
		return nil, nil
	}

	path, err := GetGCSPathForProwJobURL(pjLog, pj.Status.URL)
	if err != nil {
		pjLog.WithError(err).WithField("prowJobURL", pj.Status.URL).Error("error getting GCS path for prow job URL")
		return nil, err
	}

	bkt := pl.gcsClient.Bucket(pj.Spec.DecorationConfig.GCSConfiguration.Bucket)
	gcsJobRun := gcs.NewGCSJobRun(bkt, path)
	junitMatches, err := gcsJobRun.FindAllMatches(ctx, gcs.GlobJunitXML)
	if err != nil {
		return nil, errors.Wrap(err, "error finding junit files")
	}

	result, err := pl.buildJobRunResult(ctx, pj, id, path, junitMatches, dbProwJob)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// prowJobRunTestRow holds raw JUnit test data before ID resolution.
type prowJobRunTestRow struct {
	ProwJobRunID        uint
	ProwJobID           uint
	ProwJobRunTimestamp time.Time
	ProwJobRunRelease   string
	TestName            string
	SuiteName           string
	Status              int
	Duration            float64
	Output              *string
}

type prowJobRunRow struct {
	ID             uint
	Cluster        string
	Duration       time.Duration
	ProwJobID      uint
	ProwJobRelease string
	URL            string
	GCSBucket      string
	Timestamp      time.Time
	OverallResult  sippyprocessingv1.JobOverallResult
	TestFailures   int
	Succeeded      bool
	Labels         []string
}

type annotationRow struct {
	ProwJobRunID        uint
	Key                 string
	Value               string
	ProwJobRunRelease   string
	ProwJobRunTimestamp time.Time
}

type pullRequestRow struct {
	Org      string
	Repo     string
	Link     string
	SHA      string
	Author   string
	Title    string
	Number   int
	MergedAt *time.Time
}

type pullRequestAssocRow struct {
	ProwJobRunID        uint
	Link                string
	SHA                 string
	ProwJobRunRelease   string
	ProwJobRunTimestamp time.Time
}

type jobRunResult struct {
	Run              prowJobRunRow
	Annotations      []annotationRow
	PullRequests     []pullRequestRow
	PullRequestAssoc []pullRequestAssocRow
	Tests            []prowJobRunTestRow
}

func (pl *ProwLoader) buildJobRunResult(ctx context.Context, pj *prow.ProwJob, id uint64, path string, junitMatches []string, dbProwJob *models.ProwJob) (*jobRunResult, error) {
	tests, failures, overallResult, err := pl.prowJobRunTestsFromGCS(ctx, pj, uint(id), dbProwJob.ID, dbProwJob.Release, path, junitMatches)
	if err != nil {
		return nil, err
	}

	pulls := pl.fetchPullRequestData(pj.Spec.Refs, path)

	var pullAssocs []pullRequestAssocRow
	for _, pull := range pulls {
		pullAssocs = append(pullAssocs, pullRequestAssocRow{
			ProwJobRunID:        uint(id),
			Link:                pull.Link,
			SHA:                 pull.SHA,
			ProwJobRunRelease:   dbProwJob.Release,
			ProwJobRunTimestamp: pj.Status.StartTime,
		})
	}

	var annotations []annotationRow
	for k, v := range pj.Annotations {
		annotations = append(annotations, annotationRow{
			ProwJobRunID:        uint(id),
			Key:                 k,
			Value:               v,
			ProwJobRunRelease:   dbProwJob.Release,
			ProwJobRunTimestamp: pj.Status.StartTime,
		})
	}

	var duration time.Duration
	if pj.Status.CompletionTime != nil {
		duration = pj.Status.CompletionTime.Sub(pj.Status.StartTime)
	}

	return &jobRunResult{
		Run: prowJobRunRow{
			ID:             uint(id),
			Cluster:        pj.Spec.Cluster,
			Duration:       duration,
			ProwJobID:      dbProwJob.ID,
			ProwJobRelease: dbProwJob.Release,
			URL:            pj.Status.URL,
			GCSBucket:      pj.Spec.DecorationConfig.GCSConfiguration.Bucket,
			Timestamp:      pj.Status.StartTime,
			OverallResult:  overallResult,
			TestFailures:   failures,
			Succeeded:      overallResult == sippyprocessingv1.JobSucceeded,
			Labels:         []string(pl.labelsCache[pj.Status.BuildID]),
		},
		Annotations:      annotations,
		PullRequests:     pulls,
		PullRequestAssoc: pullAssocs,
		Tests:            tests,
	}, nil
}

var (
	runCols = []db.TempColumn[prowJobRunRow]{
		{Name: "id", Type: "bigint NOT NULL", Value: func(r *prowJobRunRow) any { return r.ID }},
		{Name: "cluster", Type: "text NOT NULL DEFAULT ''", Value: func(r *prowJobRunRow) any { return r.Cluster }},
		{Name: "duration", Type: "bigint NOT NULL DEFAULT 0", Value: func(r *prowJobRunRow) any { return int64(r.Duration) }},
		{Name: "prow_job_id", Type: "bigint NOT NULL", Value: func(r *prowJobRunRow) any { return r.ProwJobID }},
		{Name: "prow_job_release", Type: "text NOT NULL", Value: func(r *prowJobRunRow) any { return r.ProwJobRelease }},
		{Name: "url", Type: "text NOT NULL DEFAULT ''", Value: func(r *prowJobRunRow) any { return r.URL }},
		{Name: "gcs_bucket", Type: "text NOT NULL DEFAULT ''", Value: func(r *prowJobRunRow) any { return r.GCSBucket }},
		{Name: "timestamp", Type: "timestamptz NOT NULL", Value: func(r *prowJobRunRow) any { return r.Timestamp }},
		{Name: "overall_result", Type: "text NOT NULL DEFAULT ''", Value: func(r *prowJobRunRow) any { return string(r.OverallResult) }},
		{Name: "test_failures", Type: "integer NOT NULL DEFAULT 0", Value: func(r *prowJobRunRow) any { return r.TestFailures }},
		{Name: "succeeded", Type: "boolean NOT NULL DEFAULT false", Value: func(r *prowJobRunRow) any { return r.Succeeded }},
		{Name: "labels", Type: "text[]", Value: func(r *prowJobRunRow) any { return r.Labels }},
	}
	annCols = []db.TempColumn[annotationRow]{
		{Name: "prow_job_run_id", Type: "bigint NOT NULL", Value: func(a *annotationRow) any { return a.ProwJobRunID }},
		{Name: "key", Type: "text NOT NULL", Value: func(a *annotationRow) any { return a.Key }},
		{Name: "value", Type: "text NOT NULL DEFAULT ''", Value: func(a *annotationRow) any { return a.Value }},
		{Name: "prow_job_run_release", Type: "text NOT NULL", Value: func(a *annotationRow) any { return a.ProwJobRunRelease }},
		{Name: "prow_job_run_timestamp", Type: "timestamptz NOT NULL", Value: func(a *annotationRow) any { return a.ProwJobRunTimestamp }},
	}
	prCols = []db.TempColumn[pullRequestRow]{
		{Name: "org", Type: "text NOT NULL", Value: func(p *pullRequestRow) any { return p.Org }},
		{Name: "repo", Type: "text NOT NULL", Value: func(p *pullRequestRow) any { return p.Repo }},
		{Name: "link", Type: "text NOT NULL", Value: func(p *pullRequestRow) any { return p.Link }},
		{Name: "sha", Type: "text NOT NULL", Value: func(p *pullRequestRow) any { return p.SHA }},
		{Name: "author", Type: "text NOT NULL DEFAULT ''", Value: func(p *pullRequestRow) any { return p.Author }},
		{Name: "title", Type: "text NOT NULL DEFAULT ''", Value: func(p *pullRequestRow) any { return p.Title }},
		{Name: "number", Type: "integer NOT NULL DEFAULT 0", Value: func(p *pullRequestRow) any { return p.Number }},
		{Name: "merged_at", Type: "timestamptz", Value: func(p *pullRequestRow) any { return p.MergedAt }},
	}
	prAssocCols = []db.TempColumn[pullRequestAssocRow]{
		{Name: "prow_job_run_id", Type: "bigint NOT NULL", Value: func(p *pullRequestAssocRow) any { return p.ProwJobRunID }},
		{Name: "link", Type: "text NOT NULL", Value: func(p *pullRequestAssocRow) any { return p.Link }},
		{Name: "sha", Type: "text NOT NULL", Value: func(p *pullRequestAssocRow) any { return p.SHA }},
		{Name: "prow_job_run_release", Type: "text NOT NULL", Value: func(p *pullRequestAssocRow) any { return p.ProwJobRunRelease }},
		{Name: "prow_job_run_timestamp", Type: "timestamptz NOT NULL", Value: func(p *pullRequestAssocRow) any { return p.ProwJobRunTimestamp }},
	}
	testCols = []db.TempColumn[prowJobRunTestRow]{
		{Name: "prow_job_run_id", Type: "bigint NOT NULL", Value: func(r *prowJobRunTestRow) any { return r.ProwJobRunID }},
		{Name: "prow_job_id", Type: "bigint NOT NULL", Value: func(r *prowJobRunTestRow) any { return r.ProwJobID }},
		{Name: "prow_job_run_timestamp", Type: "timestamptz NOT NULL", Value: func(r *prowJobRunTestRow) any { return r.ProwJobRunTimestamp }},
		{Name: "prow_job_run_release", Type: "text NOT NULL", Value: func(r *prowJobRunTestRow) any { return r.ProwJobRunRelease }},
		{Name: "test_name", Type: "text NOT NULL", Value: func(r *prowJobRunTestRow) any { return r.TestName }},
		{Name: "suite_name", Type: "text NOT NULL DEFAULT ''", Value: func(r *prowJobRunTestRow) any { return r.SuiteName }},
		{Name: "status", Type: "integer NOT NULL", Value: func(r *prowJobRunTestRow) any { return r.Status }},
		{Name: "duration", Type: "double precision NOT NULL DEFAULT 0", Value: func(r *prowJobRunTestRow) any { return r.Duration }},
		{Name: "output", Type: "text", Value: func(r *prowJobRunTestRow) any { return r.Output }},
	}
)

func (pl *ProwLoader) accumulateAndWriteJobRuns(ctx context.Context, results <-chan *jobRunResult) {
	const flushThreshold = 100
	var (
		batch  []jobRunResult
		total  int
		failed int
	)

	flush := func(msg string) {
		if err := pl.writeJobRunBatch(ctx, batch); err != nil {
			log.WithError(err).WithField("batchSize", len(batch)).Warning(msg)
			failed += len(batch)
			pl.errors = append(pl.errors, fmt.Errorf("error writing job run batch: %w", err))
		} else {
			total += len(batch)
		}
		batch = batch[:0]
	}

	for result := range results {
		batch = append(batch, *result)
		if ctx.Err() != nil {
			break
		}
		if len(batch) >= flushThreshold {
			flush("batch write failed, continuing with remaining batches")
		}
	}
	if len(batch) > 0 {
		flush("final batch write failed")
	}

	if total > 0 || failed > 0 {
		entry := log.WithField("succeeded", total).WithField("failed", failed)
		if failed > 0 {
			entry.Warning("job run batch processing completed with errors")
		} else {
			entry.Info("all job run batches committed")
		}
	}
	prowLoaderProcessedMetricGauge.Set(float64(total))
}

func (pl *ProwLoader) writeJobRunBatch(ctx context.Context, batch []jobRunResult) error {
	if len(batch) == 0 {
		return nil
	}

	sqlDB, err := pl.dbc.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	var runs []prowJobRunRow
	var anns []annotationRow
	var prs []pullRequestRow
	var prAssocs []pullRequestAssocRow
	var tests []prowJobRunTestRow
	for i := range batch {
		runs = append(runs, batch[i].Run)
		anns = append(anns, batch[i].Annotations...)
		prs = append(prs, batch[i].PullRequests...)
		prAssocs = append(prAssocs, batch[i].PullRequestAssoc...)
		tests = append(tests, batch[i].Tests...)
	}

	cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_prow_job_runs", runs, runCols)
	if err != nil {
		return err
	}
	defer cleanup()

	if len(anns) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_annotations", anns, annCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	if len(prs) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_pull_requests", prs, prCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	if len(prAssocs) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_pr_assocs", prAssocs, prAssocCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	if len(tests) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_job_run_tests", tests, testCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `
		INSERT INTO prow_job_runs (id, cluster, duration, prow_job_id, prow_job_release,
			url, gcs_bucket, timestamp, overall_result, test_failures, succeeded,
			failed, infrastructure_failure, known_failure, labels, created_at, updated_at)
		SELECT id, cluster, duration, prow_job_id, prow_job_release,
			url, gcs_bucket, timestamp, overall_result, test_failures, succeeded,
			false, false, false, labels, NOW(), NOW()
		FROM tmp_prow_job_runs
	`); err != nil {
		return fmt.Errorf("inserting prow_job_runs: %w", err)
	}

	if len(anns) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO prow_job_run_annotations (prow_job_run_id, key, value,
				prow_job_run_release, prow_job_run_timestamp, created_at, updated_at)
			SELECT prow_job_run_id, key, value, prow_job_run_release, prow_job_run_timestamp, NOW(), NOW()
			FROM tmp_annotations
		`); err != nil {
			return fmt.Errorf("inserting prow_job_run_annotations: %w", err)
		}
	}

	if len(prs) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO prow_pull_requests (org, repo, link, sha, author, title, number, merged_at, created_at, updated_at)
			SELECT DISTINCT ON (link, sha) org, repo, link, sha, author, title, number, merged_at, NOW(), NOW()
			FROM tmp_pull_requests ORDER BY link, sha, merged_at DESC NULLS LAST
			ON CONFLICT (link, sha) DO UPDATE SET
				merged_at = COALESCE(EXCLUDED.merged_at, prow_pull_requests.merged_at),
				author = CASE WHEN prow_pull_requests.author = '' THEN EXCLUDED.author ELSE prow_pull_requests.author END,
				title = CASE WHEN prow_pull_requests.title = '' THEN EXCLUDED.title ELSE prow_pull_requests.title END,
				updated_at = NOW()
		`); err != nil {
			return fmt.Errorf("upserting prow_pull_requests: %w", err)
		}
	}

	if len(prAssocs) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO prow_job_run_prow_pull_requests (prow_job_run_id, prow_pull_request_id,
				prow_job_run_release, prow_job_run_timestamp)
			SELECT tmp.prow_job_run_id, pp.id, tmp.prow_job_run_release, tmp.prow_job_run_timestamp
			FROM tmp_pr_assocs tmp
			INNER JOIN prow_pull_requests pp ON pp.link = tmp.link AND pp.sha = tmp.sha
		`); err != nil {
			return fmt.Errorf("inserting prow_job_run_prow_pull_requests: %w", err)
		}
	}

	if len(tests) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tests (name, created_at, updated_at)
			SELECT DISTINCT test_name, NOW(), NOW() FROM tmp_job_run_tests
			ON CONFLICT (name) DO UPDATE SET deleted_at = NULL, updated_at = NOW()
			WHERE tests.deleted_at IS NOT NULL
		`); err != nil {
			return fmt.Errorf("ensuring tests exist: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO suites (name, created_at, updated_at)
			SELECT DISTINCT suite_name, NOW(), NOW() FROM tmp_job_run_tests
			WHERE suite_name != ''
			ON CONFLICT (name) DO UPDATE SET deleted_at = NULL, updated_at = NOW()
			WHERE suites.deleted_at IS NOT NULL
		`); err != nil {
			return fmt.Errorf("ensuring suites exist: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			WITH inserted AS (
				INSERT INTO prow_job_run_tests (prow_job_run_id, prow_job_id, prow_job_run_timestamp,
					prow_job_run_release, test_id, suite_id, status, duration, created_at, updated_at)
				SELECT tmp.prow_job_run_id, tmp.prow_job_id, tmp.prow_job_run_timestamp,
					tmp.prow_job_run_release, t.id, s.id, tmp.status, tmp.duration, NOW(), NOW()
				FROM tmp_job_run_tests tmp
				INNER JOIN tests t ON t.name = tmp.test_name AND t.deleted_at IS NULL
				LEFT JOIN suites s ON s.name = tmp.suite_name AND s.deleted_at IS NULL
				RETURNING id, test_id, suite_id,
					prow_job_run_id, prow_job_run_timestamp, prow_job_run_release
			)
			INSERT INTO prow_job_run_test_outputs (prow_job_run_test_id, prow_job_run_test_timestamp,
				prow_job_run_test_release, output, created_at, updated_at)
			SELECT ins.id, ins.prow_job_run_timestamp, ins.prow_job_run_release, tmp.output, NOW(), NOW()
			FROM inserted ins
			JOIN tests t ON t.id = ins.test_id
			JOIN tmp_job_run_tests tmp ON tmp.test_name = t.name AND tmp.prow_job_run_id = ins.prow_job_run_id
			LEFT JOIN suites s2 ON s2.name = tmp.suite_name AND s2.deleted_at IS NULL
			WHERE tmp.output IS NOT NULL
				AND ins.suite_id IS NOT DISTINCT FROM s2.id
		`); err != nil {
			return fmt.Errorf("inserting prow_job_run_tests: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing job run batch: %w", err)
	}

	log.WithFields(log.Fields{
		"runs":  len(batch),
		"tests": len(tests),
	}).Info("job run batch committed")

	return nil
}

func GetGCSPathForProwJobURL(pjLog log.FieldLogger, prowJobURL string) (string, error) {
	// this err validation has moved up
	// and will exit before we save / update the ProwJob
	// now, any concerns?
	pjURL, err := url.Parse(prowJobURL)
	if err != nil {
		return "", err
	}

	// Get the path in the gcs bucket, strip out the bucket name and anything before it
	path := gcsPathStrip.ReplaceAllString(pjURL.Path, "")
	pjLog.Debugf("gcs bucket path: %+v", path)
	if path == "" || len(path) == len(pjURL.Path) {
		return "", fmt.Errorf("not continuing, gcs path empty or does not contain expected prefix original=%+v stripped=%+v", pjURL.Path, path)
	}

	return path, nil
}

func (pl *ProwLoader) fetchPullRequestData(refs *prow.Refs, pjPath string) []pullRequestRow {
	if refs == nil || pl.githubClient == nil {
		return nil
	}

	var pulls []pullRequestRow
	for _, pr := range refs.Pulls {
		mergedAt, err := pl.githubClient.GetPRSHAMerged(refs.Org, refs.Repo, pr.Number, pr.SHA)
		if err != nil {
			log.WithError(err).Warningf("could not fetch pull request status from GitHub; org=%q repo=%q number=%q sha=%q", refs.Org, refs.Repo, pr.Number, pr.SHA)
		} else {
			if pr.Title == "" {
				ghTitle, err := pl.githubClient.GetPRTitle(refs.Org, refs.Repo, pr.Number)
				if err != nil {
					log.WithError(err).Warningf("could not fetch pull request title from GitHub; org=%q repo=%q number=%q sha=%q", refs.Org, refs.Repo, pr.Number, pr.SHA)
				} else if ghTitle != nil {
					pr.Title = *ghTitle
				}
			}
			if pr.Link == "" {
				ghLink, err := pl.githubClient.GetPRURL(refs.Org, refs.Repo, pr.Number)
				if err != nil {
					log.WithError(err).Warningf("could not fetch pull request url from GitHub; org=%q repo=%q number=%q sha=%q", refs.Org, refs.Repo, pr.Number, pr.SHA)
				} else if ghLink != nil {
					pr.Link = *ghLink
				}
			}
		}

		if pr.Link == "" {
			log.WithField("sha", pr.SHA).Debug("skipping pull request with empty link")
			continue
		}

		pl.ghCommenter.UpdatePendingCommentRecords(refs.Org, refs.Repo, pr.Number, pr.SHA, models.CommentTypeRiskAnalysis, mergedAt, pjPath)

		pulls = append(pulls, pullRequestRow{
			Org:      refs.Org,
			Repo:     refs.Repo,
			Link:     pr.Link,
			SHA:      pr.SHA,
			Author:   pr.Author,
			Title:    pr.Title,
			Number:   pr.Number,
			MergedAt: mergedAt,
		})
	}

	return pulls
}

func (pl *ProwLoader) prefetchLabels(prowJobs []prow.ProwJob) (map[string]pq.StringArray, error) {
	buildIDs := make([]string, 0, len(prowJobs))
	var earliest time.Time
	for i := range prowJobs {
		buildIDs = append(buildIDs, prowJobs[i].Status.BuildID)
		if earliest.IsZero() || prowJobs[i].Status.StartTime.Before(earliest) {
			earliest = prowJobs[i].Status.StartTime
		}
	}

	log.WithField("count", len(buildIDs)).Info("pre-fetching labels from BigQuery in bulk")
	start := time.Now()
	labels, err := GatherLabelsFromBQ(pl.ctx, pl.bigQueryClient, buildIDs, earliest)
	if err != nil {
		return nil, fmt.Errorf("pre-fetching %d labels from BigQuery: %w", len(buildIDs), err)
	}
	log.WithField("count", len(labels)).WithField("duration", time.Since(start)).Info("pre-fetched labels from BigQuery")
	return labels, nil
}

const LabelsDatasetEnv = "JOB_LABELS_DATASET"
const LabelsTableName = "job_labels"

// BigQuery HTTP request body limit is ~10MB; 50k build IDs stays well under that.
const labelsBatchSize = 50000

// GatherLabelsFromBQ queries BigQuery for labels for multiple job runs.
// Large ID lists are automatically batched to avoid exceeding BigQuery's request size limit.
// The startTime is used to constrain the scan to recent date partitions.
// Returns a map of buildID → labels. If a batch fails, the returned map contains
// labels from previously completed batches and the error is also returned.
func GatherLabelsFromBQ(ctx context.Context, bqClient *bqcachedclient.Client, buildIDs []string, startTime time.Time) (map[string]pq.StringArray, error) {
	if bqClient == nil || len(buildIDs) == 0 {
		return nil, nil
	}

	result := make(map[string]pq.StringArray, len(buildIDs))
	totalBatches := (len(buildIDs) + labelsBatchSize - 1) / labelsBatchSize

	for i := 0; i < len(buildIDs); i += labelsBatchSize {
		batch := buildIDs[i:min(i+labelsBatchSize, len(buildIDs))]
		batchNum := i/labelsBatchSize + 1

		log.WithField("batch", batchNum).WithField("totalBatches", totalBatches).WithField("batchSize", len(batch)).Info("querying BigQuery labels batch")

		batchResult, err := gatherLabelsBatch(ctx, bqClient, batch, startTime)
		if err != nil {
			return result, err
		}
		maps.Copy(result, batchResult)
	}

	return result, nil
}

func gatherLabelsBatch(ctx context.Context, bqClient *bqcachedclient.Client, buildIDs []string, startTime time.Time) (map[string]pq.StringArray, error) {
	dataset := os.Getenv(LabelsDatasetEnv)
	if dataset == "" {
		dataset = bqClient.Dataset
	}
	table := fmt.Sprintf("`%s.%s`", dataset, LabelsTableName)
	q := bqClient.Query(ctx, bqlabel.ProwLoaderJobLabels, `
		SELECT prowjob_build_id, ARRAY_AGG(DISTINCT label ORDER BY label ASC) AS labels
		FROM `+table+`
		WHERE prowjob_build_id IN UNNEST(@BuildIDs)
		  AND DATE(prowjob_start) >= DATE(@ReleaseTime)
		GROUP BY prowjob_build_id
	`)
	q.Parameters = []bigquery.QueryParameter{
		{
			Name:  "BuildIDs",
			Value: buildIDs,
		},
		{
			Name:  "ReleaseTime",
			Value: startTime,
		},
	}

	type row struct {
		BuildID string   `bigquery:"prowjob_build_id"`
		Labels  []string `bigquery:"labels"`
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("bulk labels query from BigQuery for %d build IDs: %w", len(buildIDs), err)
	}

	result := make(map[string]pq.StringArray, len(buildIDs))
	for {
		var r row
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return result, fmt.Errorf("bulk labels iteration from BigQuery at buildID %s: %w", r.BuildID, err)
		}
		result[r.BuildID] = r.Labels
	}

	return result, nil
}

type testCaseKey struct {
	SuiteName string
	TestName  string
}

// testCaseEntry holds raw test case data with string names before ID resolution.
type testCaseEntry struct {
	TestName  string
	SuiteName string
	Status    int
	Duration  float64
	Output    *string
}

func (pl *ProwLoader) prowJobRunTestsFromGCS(ctx context.Context, pj *prow.ProwJob, id, prowJobID uint, prowJobRelease, path string, junitPaths []string) ([]prowJobRunTestRow, int, sippyprocessingv1.JobOverallResult, error) {
	bkt := pl.gcsClient.Bucket(pj.Spec.DecorationConfig.GCSConfiguration.Bucket)
	gcsJobRun := gcs.NewGCSJobRun(bkt, path)
	gcsJobRun.SetGCSJunitPaths(junitPaths)
	suites, err := gcsJobRun.GetCombinedJUnitTestSuites(ctx)
	if err != nil {
		log.Warningf("failed to get junit test suites: %s", err.Error())
		return nil, 0, "", err
	}

	testCases := make(map[testCaseKey]*testCaseEntry)
	for _, suite := range suites.Suites {
		if !db.IsSuiteImportable(suite.Name) {
			log.Infof("skipping suite %q as it's not listed for import", suite.Name)
			continue
		}
		extractTestCases(suite, testCases)
	}

	oldTestCases := make(map[string]*models.ProwJobRunTest, len(testCases))
	for _, tc := range testCases {
		oldTestCases[tc.TestName] = &models.ProwJobRunTest{
			Status: tc.Status,
		}
	}
	syntheticSuite, jobResult := testconversion.ConvertProwJobRunToSyntheticTests(*pj, oldTestCases, pl.syntheticTestManager)

	if !db.IsSuiteImportable(syntheticSuite.Name) {
		return nil, 0, "", fmt.Errorf("synthetic suite %q is missing from the importable list", syntheticSuite.Name)
	}
	extractTestCases(syntheticSuite, testCases)
	log.Infof("synthetic suite had %d tests", syntheticSuite.NumTests)

	failures := 0
	results := make([]prowJobRunTestRow, 0, len(testCases))
	for _, tc := range testCases {
		if testidentification.IsIgnoredTest(tc.TestName) {
			continue
		}
		results = append(results, prowJobRunTestRow{
			ProwJobRunID:        id,
			ProwJobID:           prowJobID,
			ProwJobRunTimestamp: pj.Status.StartTime,
			ProwJobRunRelease:   prowJobRelease,
			TestName:            tc.TestName,
			SuiteName:           tc.SuiteName,
			Status:              tc.Status,
			Duration:            tc.Duration,
			Output:              tc.Output,
		})
		if tc.Status == int(sippyprocessingv1.TestStatusFailure) {
			failures++
		}
	}

	return results, failures, jobResult, nil
}

func extractTestCases(suite *junit.TestSuite, testCases map[testCaseKey]*testCaseEntry) {
	for _, tc := range suite.TestCases {
		if testidentification.IsIgnoredTest(tc.Name) {
			continue
		}
		status := sippyprocessingv1.TestStatusFailure
		var output *string
		switch {
		case tc.SkipMessage != nil:
			continue
		case tc.FailureOutput == nil:
			status = sippyprocessingv1.TestStatusSuccess
		default:
			output = &tc.FailureOutput.Output
		}

		key := testCaseKey{SuiteName: suite.Name, TestName: tc.Name}

		if existing, ok := testCases[key]; !ok {
			testCases[key] = &testCaseEntry{
				TestName:  tc.Name,
				SuiteName: suite.Name,
				Status:    int(status),
				Duration:  tc.Duration,
				Output:    output,
			}
		} else if (existing.Status == int(sippyprocessingv1.TestStatusFailure) && status == sippyprocessingv1.TestStatusSuccess) ||
			(existing.Status == int(sippyprocessingv1.TestStatusSuccess) && status == sippyprocessingv1.TestStatusFailure) {
			existing.Status = int(sippyprocessingv1.TestStatusFlake)
			if existing.Output == nil {
				existing.Output = output
			}
		}
	}

	for _, c := range suite.Children {
		extractTestCases(c, testCases)
	}
}
