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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/util/sets"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/testconversion"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/types"
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
	ctx                  context.Context
	dbc                  *db.DB
	errors               []error
	githubClient         *github.Client
	bigQueryClient       *bqcachedclient.Client
	maxConcurrency       int
	prowJobCache         map[string]*models.ProwJob
	variantManager       testidentification.VariantManager
	syntheticTestManager synthetictests.SyntheticTestManager
	releases             []string
	releaseAttributor    *ReleaseAttributor
	config               *v1config.SippyConfig
	ghCommenter          *commenter.GitHubCommenter
	gcsClient            *storage.Client
	promPusher           *push.Pusher
	loadSince            *time.Time
	labelsCache          map[string]pq.StringArray
	currentDate          civil.Date
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

	releaseAttributor := NewReleaseAttributor(releases, config, syntheticReleaseJobOverrides)
	return &ProwLoader{
		ctx:                  ctx,
		dbc:                  dbc,
		gcsClient:            gcsClient,
		githubClient:         githubClient,
		bigQueryClient:       bigQueryClient,
		maxConcurrency:       50,
		syntheticTestManager: syntheticTestManager,
		variantManager:       variantManager,
		releases:             releases,
		releaseAttributor:    releaseAttributor,
		config:               config,
		ghCommenter:          ghCommenter,
		promPusher:           promPusher,
		loadSince:            loadSince,
		currentDate:          civil.DateOf(time.Now().UTC()),
	}
}

func (pl *ProwLoader) matchRelease(pj *prow.ProwJob) string {
	return pl.releaseAttributor.Match(pj)
}

const DefaultLookbackDays = 14

func resolveFrom(since *time.Time, to time.Time) time.Time {
	if since != nil {
		return *since
	}
	return to.AddDate(0, 0, -DefaultLookbackDays)
}

func (pl *ProwLoader) resolveLoadSince() time.Time {
	return resolveFrom(pl.loadSince, time.Now().UTC())
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

// partitionStartDate computes the start of the date range for which partitions must
// exist to accommodate the given prowJobs. loadSince (minus a 1 day grace period, since
// bq imports based on modified time which can include job_run_start_time a day earlier,
// see https://github.com/openshift/sippy/blob/main/pkg/dataloader/prowloader/prow.go#L473)
// is the default start date, extended earlier if any job's StartTime precedes it.
//
// That grace period covers the common case, but some jobs (e.g. ones Prow eventually marks
// as aborted after getting stuck) can report a StartTime days before their completion time,
// which is what the BQ query actually filters on. So the start bound is extended to the
// earliest job we're actually about to write, to guarantee a partition exists for every row.
func partitionStartDate(loadSince time.Time, prowJobs []prow.ProwJob) time.Time {
	startDate := loadSince.AddDate(0, 0, -1)
	for i := range prowJobs {
		if st := prowJobs[i].Status.StartTime; !st.IsZero() && st.Before(startDate) {
			startDate = st
		}
	}
	return startDate
}

// ensurePartitions creates necessary partitions for partitioned tables.
// It uses the release list from pl.releases and determines the date range based on:
//   - pl.loadSince if available, otherwise looks back DefaultLookbackDays days, plus a 1 day grace period
//   - the earliest prowJobs StartTime, in case it falls outside the above window
//   - Creates partitions 2 days forward from now
func (pl *ProwLoader) ensurePartitions(prowJobs []prow.ProwJob) error {
	defaultStartDate := pl.resolveLoadSince().AddDate(0, 0, -1)
	startDate := partitionStartDate(pl.resolveLoadSince(), prowJobs)
	if startDate.Before(defaultStartDate) {
		log.Warnf("extending partition start date to %s to cover outlier job StartTime (default was %s)",
			startDate.Format("2006-01-02"), defaultStartDate.Format("2006-01-02"))
	}

	// Create partitions 2 days forward from now
	endDate := time.Now().UTC().AddDate(0, 0, 2)

	log.Infof("Ensuring partitions for releases %v from %s to %s",
		pl.releases, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	count, err := pl.dbc.EnsurePartitions(pl.releases, startDate, endDate, false)
	if err != nil {
		return fmt.Errorf("failed to ensure partitions: %w", err)
	}

	log.Infof("Ensured %d partitions across all partitioned tables", count)
	return nil
}

func (pl *ProwLoader) Load() {
	start := time.Now()

	log.Infof("started loading prow jobs to DB...")

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
	if err := pl.ensurePartitions(prowJobs); err != nil {
		pl.errors = append(pl.errors, errors.Wrap(err, "failed to ensure partitions"))
		return
	}

	// Clean up old partitions (detach partitions older than 100 days, drop detached partitions older than 110 days)
	if detached, dropped, err := pl.dbc.CleanupPartitions(false); err != nil {
		log.WithError(err).Warning("failed to cleanup old partitions, continuing with load")
	} else {
		log.Infof("Partition cleanup complete: detached %d, dropped %d", detached, dropped)
	}

	// Carry forward cumulative summaries to tomorrow for any release that
	// doesn't already have data for that date. This must run after
	// ensurePartitions so the target partition exists.
	if err := pgwriter.CarryForwardCumulativeSummaries(pl.ctx, pl.dbc, pl.currentDate, pl.releases); err != nil {
		pl.errors = append(pl.errors, errors.Wrap(err, "error in cumulative summary carry-forward"))
		return
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
	results := make(chan *pgwriter.JobRunResult, len(entries))
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

	if len(pl.errors) > 0 {
		log.Warningf("encountered %d errors while importing job runs", len(pl.errors))
	}
	log.Infof("finished importing new job runs in %+v", time.Since(start))

	if pl.promPusher != nil {
		pl.promPusher.Collector(prowLoaderQueriedMetricGauge)
		pl.promPusher.Collector(prowLoaderProcessedMetricGauge)
	}
}

// isPayloadPresubmit returns true if the prow job is a /payload sub-job.
func isPayloadPresubmit(pj *prow.ProwJob) bool {
	_, hasAnnotation := pj.Annotations["releaseJobName"]
	return hasAnnotation && pj.Spec.Refs != nil
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

		release := pl.matchRelease(pj)
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

		variantJobName := pj.Spec.Job
		isPayload := isPayloadPresubmit(pj)
		if isPayload {
			variantJobName = pj.Annotations["releaseJobName"]
		}

		variants := pl.variantManager.IdentifyVariants(variantJobName)
		if isPayload {
			for vi, v := range variants {
				parts := strings.SplitN(v, ":", 2)
				if len(parts) == 2 {
					if _, isRel := pl.config.Releases[parts[1]]; isRel {
						variants[vi] = parts[0] + ":" + models.ReleasePresubmits
						break
					}
				}
			}
		}

		testGridURL := ""
		if !isPayload {
			testGridURL = pl.generateTestGridURL(release, pj.Spec.Job).String()
		}

		jobDefs = append(jobDefs, models.ProwJob{
			Name:        pj.Spec.Job,
			Kind:        models.ProwKind(pj.Spec.Type),
			Release:     release,
			Variants:    variants,
			TestGridURL: testGridURL,
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
		LEFT JOIN prow_job_run_id_map m ON m.id = t.id
		WHERE m.id IS NULL
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

func (pl *ProwLoader) fetchJobRunResult(ctx context.Context, pj *prow.ProwJob) (*pgwriter.JobRunResult, error) {
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

func (pl *ProwLoader) buildJobRunResult(ctx context.Context, pj *prow.ProwJob, id uint64, path string, junitMatches []string, dbProwJob *models.ProwJob) (*pgwriter.JobRunResult, error) {
	tests, failures, flakes, overallResult, err := pl.prowJobRunTestsFromGCS(ctx, pj, uint(id), dbProwJob.ID, dbProwJob.Release, path, junitMatches)
	if err != nil {
		return nil, err
	}

	pulls := pl.fetchPullRequestData(pj.Spec.Refs, path)

	var pullAssocs []pgwriter.PullRequestAssocRow
	for _, pull := range pulls {
		pullAssocs = append(pullAssocs, pgwriter.PullRequestAssocRow{
			ProwJobRunID:        uint(id),
			Link:                pull.Link,
			SHA:                 pull.SHA,
			ProwJobRunRelease:   dbProwJob.Release,
			ProwJobRunTimestamp: pj.Status.StartTime,
		})
	}

	var annotations []pgwriter.AnnotationRow
	for k, v := range pj.Annotations {
		annotations = append(annotations, pgwriter.AnnotationRow{
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

	return &pgwriter.JobRunResult{
		Run: pgwriter.RunRow{
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
			TestFlakes:     flakes,
			Succeeded:      overallResult == sippyprocessingv1.JobSucceeded,
			Labels:         []string(pl.labelsCache[pj.Status.BuildID]),
		},
		Annotations:      annotations,
		PullRequests:     pulls,
		PullRequestAssoc: pullAssocs,
		Tests:            tests,
	}, nil
}

func (pl *ProwLoader) accumulateAndWriteJobRuns(ctx context.Context, results <-chan *pgwriter.JobRunResult) {
	pl.accumulateAndWrite(ctx, results, func(ctx context.Context, batch []pgwriter.JobRunResult) error {
		return pgwriter.Write(ctx, pl.dbc, pl.currentDate, batch)
	})
}

func (pl *ProwLoader) accumulateAndWrite(ctx context.Context, results <-chan *pgwriter.JobRunResult, writeBatch func(context.Context, []pgwriter.JobRunResult) error) {
	const flushThreshold = 100
	var (
		batch  []pgwriter.JobRunResult
		total  int
		failed int
	)

	flush := func(msg string) {
		if err := writeBatch(ctx, batch); err != nil {
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

func (pl *ProwLoader) fetchPullRequestData(refs *prow.Refs, pjPath string) []pgwriter.PullRequestRow {
	if refs == nil || pl.githubClient == nil {
		return nil
	}

	var pulls []pgwriter.PullRequestRow
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

		pulls = append(pulls, pgwriter.PullRequestRow{
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

func (pl *ProwLoader) prowJobRunTestsFromGCS(ctx context.Context, pj *prow.ProwJob, id, prowJobID uint, prowJobRelease, path string, junitPaths []string) ([]pgwriter.TestRow, int, int, sippyprocessingv1.JobOverallResult, error) {
	bkt := pl.gcsClient.Bucket(pj.Spec.DecorationConfig.GCSConfiguration.Bucket)
	gcsJobRun := gcs.NewGCSJobRun(bkt, path)
	gcsJobRun.SetGCSJunitPaths(junitPaths)
	suites, err := gcsJobRun.GetCombinedJUnitTestSuites(ctx)
	if err != nil {
		log.Warningf("failed to get junit test suites: %s", err.Error())
		return nil, 0, 0, "", err
	}

	testCases := make(map[testCaseKey]*types.TestCaseEntry)
	for _, suite := range suites.Suites {
		if !db.IsSuiteImportable(suite.Name) {
			log.Infof("skipping suite %q as it's not listed for import", suite.Name)
			continue
		}
		extractTestCases(suite, testCases)
	}

	oldTestCases := slices.Collect(maps.Values(testCases))
	syntheticSuite, jobResult := testconversion.ConvertProwJobRunToSyntheticTests(*pj, oldTestCases, pl.syntheticTestManager)

	if !db.IsSuiteImportable(syntheticSuite.Name) {
		return nil, 0, 0, "", fmt.Errorf("synthetic suite %q is missing from the importable list", syntheticSuite.Name)
	}
	extractTestCases(syntheticSuite, testCases)
	log.Infof("synthetic suite had %d tests", syntheticSuite.NumTests)

	failures := 0
	flakes := 0
	results := make([]pgwriter.TestRow, 0, len(testCases))
	for _, tc := range testCases {
		if testidentification.IsIgnoredTest(tc.TestName) {
			continue
		}
		results = append(results, pgwriter.TestRow{
			ProwJobRunID:        id,
			ProwJobID:           prowJobID,
			ProwJobRunTimestamp: pj.Status.StartTime,
			ProwJobRunRelease:   prowJobRelease,
			TestName:            tc.TestName,
			SuiteName:           tc.SuiteName,
			Status:              tc.Status,
			Duration:            tc.Duration,
			Output:              tc.Output,
			Lifecycle:           tc.Lifecycle,
		})
		switch tc.Status {
		case int(sippyprocessingv1.TestStatusFailure):
			failures++
		case int(sippyprocessingv1.TestStatusFlake):
			flakes++
		}
	}

	return results, failures, flakes, jobResult, nil
}

func extractTestCases(suite *junit.TestSuite, testCases map[testCaseKey]*types.TestCaseEntry) {
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
			testCases[key] = &types.TestCaseEntry{
				TestName:  tc.Name,
				SuiteName: suite.Name,
				Status:    int(status),
				Duration:  tc.Duration,
				Output:    output,
				Lifecycle: normalizeLifecycle(tc.Lifecycle),
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

// normalizeLifecycle returns the lifecycle value from JUnit XML, defaulting
// empty/missing values to "blocking" (matches BQ COALESCE behavior).
func normalizeLifecycle(raw string) string {
	if raw == "" {
		return "blocking"
	}
	return strings.ToLower(raw)
}
