package prowloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgtype"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
)

// gcsPathStrip is used to strip out everything but the path, i.e. match "/view/gs/origin-ci-test/"
// from the path "/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.14-e2e-gcp-sdn/1737420379221135360"
var gcsPathStrip = regexp.MustCompile(`.*/gs/[^/]+/`)

type ProwLoader struct {
	ctx                     context.Context
	dbc                     *db.DB
	bkt                     *storage.BucketHandle
	bktName                 string
	errors                  []error
	githubClient            *github.Client
	bigQueryClient          *bigquery.Client
	maxConcurrency          int
	prowJobCache            map[string]*models.ProwJob
	prowJobCacheLock        sync.RWMutex
	prowJobRunCache         map[uint]bool
	prowJobRunCacheLock     sync.RWMutex
	prowJobRunTestCache     map[string]uint
	prowJobRunTestCacheLock sync.RWMutex
	variantManager          testidentification.VariantManager
	suiteCache              map[string]*uint
	suiteCacheLock          sync.RWMutex
	syntheticTestManager    synthetictests.SyntheticTestManager
	releases                []string
	config                  *v1config.SippyConfig
	ghCommenter             *commenter.GitHubCommenter
	jobsImportedCount       atomic.Int32
}

func New(
	ctx context.Context,
	dbc *db.DB,
	gcsClient *storage.Client,
	bigQueryClient *bigquery.Client,
	gcsBucket string,
	githubClient *github.Client,
	variantManager testidentification.VariantManager,
	syntheticTestManager synthetictests.SyntheticTestManager,
	releases []string,
	config *v1config.SippyConfig,
	ghCommenter *commenter.GitHubCommenter) *ProwLoader {

	bkt := gcsClient.Bucket(gcsBucket)

	return &ProwLoader{
		ctx:                  ctx,
		dbc:                  dbc,
		bkt:                  bkt,
		bktName:              gcsBucket,
		githubClient:         githubClient,
		bigQueryClient:       bigQueryClient,
		maxConcurrency:       10,
		prowJobRunCache:      loadProwJobRunCache(dbc),
		prowJobCache:         loadProwJobCache(dbc),
		prowJobRunTestCache:  make(map[string]uint),
		suiteCache:           make(map[string]*uint),
		syntheticTestManager: syntheticTestManager,
		variantManager:       variantManager,
		releases:             releases,
		config:               config,
		ghCommenter:          ghCommenter,
	}
}

var clusterDataDateTimeName = regexp.MustCompile(`cluster-data_(?P<DATE>.*)-(?P<TIME>.*).json`)

type DateTimeName struct {
	Name string
	Date string
	Time string
}

func loadProwJobCache(dbc *db.DB) map[string]*models.ProwJob {
	prowJobCache := map[string]*models.ProwJob{}
	var allJobs []*models.ProwJob
	dbc.DB.Model(&models.ProwJob{}).Find(&allJobs)
	for _, j := range allJobs {
		if _, ok := prowJobCache[j.Name]; !ok {
			prowJobCache[j.Name] = j
		}
	}
	log.Infof("job cache created with %d entries from database", len(prowJobCache))
	return prowJobCache
}

// Cache the IDs of all known ProwJobRuns. Will be used to skip job run and test
// results we've already processed.
// TODO: over 800k in our db now, should we only cache those within last two weeks?
func loadProwJobRunCache(dbc *db.DB) map[uint]bool {
	prowJobRunCache := map[uint]bool{} // value is unused, just hashing
	knownJobRuns := []models.ProwJobRun{}
	ids := make([]uint, 0)
	dbc.DB.Select("id").Find(&knownJobRuns).Pluck("id", &ids)
	for _, kjr := range ids {
		prowJobRunCache[kjr] = true
	}

	return prowJobRunCache
}

func (pl *ProwLoader) Name() string {
	return "prow"
}

func (pl *ProwLoader) Errors() []error {
	return pl.errors
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

	err := pl.loadDailyTestAnalysisByJob(pl.ctx)
	if err != nil {
		pl.errors = append(pl.errors, errors.Wrap(err, "error updating daily test analysis by job"))
	}

	if true {
		log.Fatal("TODO exiting early")
		return
	}

	queue := make(chan *prow.ProwJob)
	errsCh := make(chan error, len(prowJobs))
	total := len(prowJobs)

	// Producer to keep feeding the queue
	go prowJobsProducer(pl.ctx, queue, prowJobs)

	// Start pl.maxConcurrency consumers
	var wg sync.WaitGroup
	for i := 0; i < pl.maxConcurrency; i++ {
		wg.Add(1)
		go func(ctx context.Context) {
			defer wg.Done()
			for job := range queue {
				if err := ctx.Err(); err != nil {
					errsCh <- err
					log.WithError(err).Warningf("consumer exiting, got error")
					break
				}
				if err := pl.processProwJob(ctx, job); err != nil {
					errsCh <- err
					log.WithError(err).Warningf("couldn't import job %s/%s, continuing", job.Spec.Job, job.Status.BuildID)
				}
				pl.jobsImportedCount.Add(1)
				log.Infof("%d of %d job runs processed", pl.jobsImportedCount.Load(), total)
			}
		}(pl.ctx)
	}

	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		pl.errors = append(pl.errors, err)
	}

	if len(pl.errors) > 0 {
		log.Warningf("encountered %d errors while importing job runs", len(pl.errors))
	}
	log.Infof("finished importing new job runs in %+v", time.Since(start))
}

func prowJobsProducer(ctx context.Context, queue chan *prow.ProwJob, jobs []prow.ProwJob) {
	defer close(queue)
	for i := range jobs {
		select {
		case queue <- &jobs[i]:
		case <-ctx.Done():
			return
		}
	}
}

// tempBQTestAnalysisByJobForDate is a dupe type to work around date parsing issues.
type tempBQTestAnalysisByJobForDate struct {
	Date     civil.Date
	TestID   uint
	Release  string
	TestName string
	JobName  string
	Runs     int
	Passes   int
	Flakes   int
	Failures int
}

func (pl *ProwLoader) loadDailyTestAnalysisByJob(ctx context.Context) error {

	// Figure out our last imported daily summary.
	var lastDailySummary time.Time
	row := pl.dbc.DB.Table("test_analysis_by_job_for_dates").Select("max('date')").Row()
	err := row.Scan(&lastDailySummary)
	if err != nil || lastDailySummary.IsZero() {
		log.WithError(err).Warn("no last summary found (new database?), importing last two weeks")
		lastDailySummary = time.Now().Add(-14 * 24 * time.Hour)
	}
	log.Infof("Loading test analysis by job daily summaries since: %s", lastDailySummary.UTC().Format(time.RFC3339))

	// NOTE: casting a couple datetime columns to timestamps, it does appear they go in as UTC, and thus come out
	// as the default UTC correctly.
	// Annotations and labels can be queried here if we need them.
	query := pl.bigQueryClient.Query(`WITH
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
    openshift-gce-devel.ci_analysis_us.junit
  INNER JOIN
    openshift-gce-devel.ci_analysis_us.jobs jobs
  ON
    junit.prowjob_build_id = jobs.prowjob_build_id
    AND DATE(jobs.prowjob_start) >= DATE(@From)
    AND DATE(jobs.prowjob_start) <= DATE(@To)
  WHERE
    DATE(junit.modified_time) >= DATE(@From)
    AND DATE(junit.modified_time) <= DATE(@To)
    AND test_name = 'install should succeed: overall'
    AND junit.prowjob_name = 'periodic-ci-openshift-release-master-nightly-4.18-upgrade-from-stable-4.17-e2e-metal-ipi-ovn-upgrade'
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
  AND branch = '4.18'
GROUP BY
  test_name,
  date,
  release,
  prowjob_name
ORDER BY
  date,
  test_name,
  prowjob_name
`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: "2024-10-25",
		},
		{
			Name:  "To",
			Value: "2024-10-30",
		},
	}
	it, err := query.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying test analysis from bigquery")
		return err
	}

	count := 0
	for {
		row := tempBQTestAnalysisByJobForDate{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing prowjob from bigquery")
			return err
			//continue
		}
		log.Infof("got row %+v", row)

		/*
			var refs *prow.Refs
			if bqjr.PRNumber.StringVal != "" {
				prNumber, err := strconv.Atoi(bqjr.PRNumber.StringVal)
				if err != nil {
					log.WithError(err).Errorf("Invalid pull request number from big query fetch prow jobs")
				} else {
					refs = &prow.Refs{Org: bqjr.PROrg.StringVal, Repo: bqjr.PRRepo.StringVal}
					pulls := make([]prow.Pull, 0)
					pull := prow.Pull{Number: prNumber, SHA: bqjr.PRSha.StringVal, Author: bqjr.PRAuthor.StringVal}
					refs.Pulls = append(pulls, pull)
				}
			} else if bqjr.Type == "presubmit" {
				log.Warningf("Presubmit job found without matching PR data for: %s", bqjr.JobName)
			}
			// Convert to a prow.ProwJob:
			// If we read in an invalid StartTime, skip this job but put out an error.
			if !bqjr.StartTime.Valid {
				log.WithField("job", bqjr.JobName).Error("invalid start time for prowjob")
				// Do not return an error as that will cause the job to fail.
				continue
			}
			prowJobs[bqjr.BuildID] = prow.ProwJob{
				Spec: prow.ProwJobSpec{
					Type:    bqjr.Type,
					Cluster: bqjr.Cluster,
					Job:     bqjr.JobName,
					Refs:    refs,
				},
				Status: prow.ProwJobStatus{
					StartTime:      bqjr.StartTime.Timestamp,
					CompletionTime: &bqjr.CompletionTime,
					State:          prow.ProwJobState(bqjr.State),
					URL:            bqjr.URL,
					BuildID:        bqjr.BuildID,
				},
			}

		*/
		count++
	}

	return nil
}

func (pl *ProwLoader) processProwJob(ctx context.Context, pj *prow.ProwJob) error {
	pjLog := log.WithFields(log.Fields{
		"job":     pj.Spec.Job,
		"buildID": pj.Status.BuildID,
	})

	for _, release := range pl.releases {
		cfg, ok := pl.config.Releases[release]
		if !ok {
			log.Warningf("configuration not found for release %q", release)
			continue
		}

		if val, ok := cfg.Jobs[pj.Spec.Job]; val && ok {
			if err := pl.prowJobToJobRun(ctx, pj, release); err != nil {
				err = errors.Wrapf(err, "error converting prow job to job run: %s", pj.Spec.Job)
				pjLog.WithError(err).Warning("prow import error")
				return err
			}
			return nil
		}

		for _, expr := range cfg.Regexp {
			re, err := regexp.Compile(expr)
			if err != nil {
				err = errors.Wrap(err, "invalid regex in configuration")
				log.WithError(err).Errorf("config regex error")
				continue
			}

			if re.MatchString(pj.Spec.Job) {
				if err := pl.prowJobToJobRun(ctx, pj, release); err != nil {
					err = errors.Wrapf(err, "error converting prow job to job run: %s", pj.Spec.Job)
					pjLog.WithError(err).Warning("prow import error")
					return err
				}
				return nil
			}
		}
	}

	pjLog.Debugf("no match for release in sippy configuration, skipping")
	return nil
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
			if recentMergedAt != nil {
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
		blockingJobs := sets.NewString(releaseConfig.BlockingJobs...)
		informingJobs := sets.NewString(releaseConfig.InformingJobs...)
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

func GetClusterData(ctx context.Context, bkt *storage.BucketHandle, path string, matches []string) models.ClusterData {
	cd := models.ClusterData{}
	bytes, err := GetClusterDataBytes(ctx, bkt, path, matches)
	if err != nil {
		log.WithError(err).Error("failed to get prow job variant data, returning empty cluster data and proceeding")
		return cd
	} else if bytes == nil {
		log.Warnf("empty job variant data file, returning empty cluster data and proceeding")
		return cd
	}
	err = json.Unmarshal(bytes, &cd)
	if err != nil {
		log.WithError(err).Error("failed to unmarshal cluster-data bytes, returning empty cluster data")
		return cd
	}

	return cd
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

func (pl *ProwLoader) prowJobToJobRun(ctx context.Context, pj *prow.ProwJob, release string) error {
	pjLog := log.WithFields(log.Fields{
		"job":     pj.Spec.Job,
		"buildID": pj.Status.BuildID,
		"start":   pj.Status.StartTime,
	})

	if pj.Status.State == prow.PendingState || pj.Status.State == prow.TriggeredState {
		pjLog.Infof("skipping, job not in a terminal state yet")
		return nil
	}

	id, err := strconv.ParseUint(pj.Status.BuildID, 0, 64)
	if err != nil {
		pjLog.Warningf("skipping, couldn't parse build ID: %+v", err)
		return nil
	}

	pjLog.Infof("starting processing")

	// find all files here then pass to getClusterData
	// and prowJobRunTestsFromGCS
	// add more regexes if we require more
	// results from scanning for file names
	path, err := GetGCSPathForProwJobURL(pjLog, pj.Status.URL)
	if err != nil {
		pjLog.WithError(err).WithField("prowJobURL", pj.Status.URL).Error("error getting GCS path for prow job URL")
		return err
	}
	gcsJobRun := gcs.NewGCSJobRun(pl.bkt, path)
	allMatches := gcsJobRun.FindAllMatches([]*regexp.Regexp{gcs.GetDefaultJunitFile()})
	var junitMatches []string
	if len(allMatches) > 0 {
		junitMatches = allMatches[0]
	}

	// Lock the whole prow job block to avoid trying to create the pj multiple times concurrently\
	// (resulting in a DB error)
	pl.prowJobCacheLock.Lock()
	dbProwJob, foundProwJob := pl.prowJobCache[pj.Spec.Job]
	if !foundProwJob {
		pjLog.Info("creating new ProwJob")
		dbProwJob = &models.ProwJob{
			Name:        pj.Spec.Job,
			Kind:        models.ProwKind(pj.Spec.Type),
			Release:     release,
			Variants:    pl.variantManager.IdentifyVariants(pj.Spec.Job),
			TestGridURL: pl.generateTestGridURL(release, pj.Spec.Job).String(),
		}
		err := pl.dbc.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(dbProwJob).Error
		if err != nil {
			return errors.Wrapf(err, "error loading prow job into db: %s", pj.Spec.Job)
		}
		pl.prowJobCache[pj.Spec.Job] = dbProwJob
	} else {
		saveDB := false
		newVariants := pl.variantManager.IdentifyVariants(pj.Spec.Job)
		if !reflect.DeepEqual(newVariants, []string(dbProwJob.Variants)) || dbProwJob.Kind != models.ProwKind(pj.Spec.Type) {
			dbProwJob.Kind = models.ProwKind(pj.Spec.Type)
			dbProwJob.Variants = newVariants
			saveDB = true
		}
		if len(dbProwJob.TestGridURL) == 0 {
			dbProwJob.TestGridURL = pl.generateTestGridURL(release, pj.Spec.Job).String()
			if len(dbProwJob.TestGridURL) > 0 {
				saveDB = true
			}
		}
		if saveDB {
			if res := pl.dbc.DB.WithContext(ctx).Save(&dbProwJob); res.Error != nil {
				return res.Error
			}
		}
	}
	pl.prowJobCacheLock.Unlock()

	pl.prowJobRunCacheLock.RLock()
	_, ok := pl.prowJobRunCache[uint(id)]
	pl.prowJobRunCacheLock.RUnlock()
	if ok {
		pjLog.Infof("job run was already processed")
	} else {
		pjLog.Info("processing GCS bucket")

		tests, failures, overallResult, err := pl.prowJobRunTestsFromGCS(ctx, pj, uint(id), path, junitMatches)
		if err != nil {
			return err
		}

		pulls := pl.findOrAddPullRequests(pj.Spec.Refs, path)

		var duration time.Duration
		if pj.Status.CompletionTime != nil {
			duration = pj.Status.CompletionTime.Sub(pj.Status.StartTime)
		}

		err = pl.dbc.DB.WithContext(ctx).Create(&models.ProwJobRun{
			Model: gorm.Model{
				ID: uint(id),
			},
			Cluster:       pj.Spec.Cluster,
			Duration:      duration,
			ProwJob:       *dbProwJob,
			ProwJobID:     dbProwJob.ID,
			URL:           pj.Status.URL,
			Timestamp:     pj.Status.StartTime,
			OverallResult: overallResult,
			PullRequests:  pulls,
			TestFailures:  failures,
			Succeeded:     overallResult == sippyprocessingv1.JobSucceeded,
		}).Error
		if err != nil {
			return err
		}
		// Looks like sometimes, we might be getting duplicate entries from bigquery:
		pl.prowJobRunCacheLock.Lock()
		pl.prowJobRunCache[uint(id)] = true
		pl.prowJobRunCacheLock.Unlock()

		err = pl.dbc.DB.WithContext(ctx).Debug().CreateInBatches(tests, 1000).Error
		if err != nil {
			return err
		}
	}

	pjLog.Infof("processing complete")
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
		msg := fmt.Sprintf("not continuing, gcs path empty or does not contain expected prefix original=%+v stripped=%+v", pjURL.Path, path)
		return "", fmt.Errorf(msg)
	}

	return path, nil
}

func (pl *ProwLoader) findOrAddPullRequests(refs *prow.Refs, pjPath string) []models.ProwPullRequest {
	if refs == nil || pl.githubClient == nil {
		if refs == nil {
			log.Debug("findOrAddPullRequests nil refs")
		} else {
			log.Debug("findOrAddPullRequests nil githubclient")
		}
		return nil
	}

	pulls := make([]models.ProwPullRequest, 0)

	for _, pr := range refs.Pulls {

		// title and link are not filled in via bigquery
		// so get them from github if missing

		mergedAt, err := pl.githubClient.GetPRSHAMerged(refs.Org, refs.Repo, pr.Number, pr.SHA)
		if err != nil {
			log.WithError(err).Warningf("could not fetch pull request status from GitHub; org=%q repo=%q number=%q sha=%q", refs.Org, refs.Repo, pr.Number, pr.SHA)
		} else {
			// pr should be cached from lookup above
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
			log.Debugf("findOrAddPullRequests skipping empty link for sha: %s", pr.SHA)
			continue
		}

		// any concerns if we are missing title?

		// create / update any presubmit comment records
		pl.ghCommenter.UpdatePendingCommentRecords(refs.Org, refs.Repo, pr.Number, pr.SHA, models.CommentTypeRiskAnalysis, mergedAt, pjPath)

		pull := models.ProwPullRequest{}
		res := pl.dbc.DB.Where("link = ? and sha = ?", pr.Link, pr.SHA).First(&pull)

		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			pull.MergedAt = mergedAt
			pull.Org = refs.Org
			pull.Repo = refs.Repo
			pull.Link = pr.Link
			pull.SHA = pr.SHA
			pull.Author = pr.Author
			pull.Title = pr.Title
			pull.Number = pr.Number
			res := pl.dbc.DB.Save(&pull)
			if res.Error != nil {
				log.WithError(res.Error).Warningf("could not save pull request %s (%s)", pr.Link, pr.SHA)
				continue
			}

		} else if res.Error != nil {
			log.WithError(res.Error).Errorf("unexpected error looking for pull request %s (%s)", pr.Link, pr.SHA)
			continue
		}

		if pull.MergedAt == nil || *pull.MergedAt != *mergedAt {
			pull.MergedAt = mergedAt
			if res := pl.dbc.DB.Save(pull); res.Error != nil {
				log.WithError(res.Error).Errorf("unexpected error updating pull request %s (%s)", pr.Link, pr.SHA)
				continue
			}
		}

		pulls = append(pulls, pull)
	}

	return pulls
}

func (pl *ProwLoader) findOrAddTest(name string) (uint, error) {
	pl.prowJobRunTestCacheLock.RLock()
	if id, ok := pl.prowJobRunTestCache[name]; ok {
		pl.prowJobRunTestCacheLock.RUnlock()
		return id, nil
	}
	pl.prowJobRunTestCacheLock.RUnlock()

	pl.prowJobRunTestCacheLock.Lock()
	defer pl.prowJobRunTestCacheLock.Unlock()
	test := &models.Test{}
	pl.dbc.DB.Where("name = ?", name).Find(&test)
	if test.ID == 0 {
		test.Name = name
		tx := pl.dbc.DB.Save(test)
		if tx.Error != nil {
			log.WithError(tx.Error).Warningf("failed to create test %q", name)
			return 0, tx.Error
		}
	}

	pl.prowJobRunTestCache[name] = test.ID
	return test.ID, nil
}

func (pl *ProwLoader) findSuite(name string) *uint {
	if name == "" {
		return nil
	}

	pl.suiteCacheLock.RLock()
	if id, ok := pl.suiteCache[name]; ok {
		pl.suiteCacheLock.RUnlock()
		return id
	}
	pl.suiteCacheLock.RUnlock()

	pl.suiteCacheLock.Lock()
	defer pl.suiteCacheLock.Unlock()
	suite := &models.Suite{}
	pl.dbc.DB.Where("name = ?", name).Find(&suite)
	if suite.ID == 0 {
		pl.suiteCache[name] = nil
	} else {
		id := suite.ID
		pl.suiteCache[name] = &id
	}
	return pl.suiteCache[name]
}

func (pl *ProwLoader) prowJobRunTestsFromGCS(ctx context.Context, pj *prow.ProwJob, id uint, path string, junitPaths []string) ([]*models.ProwJobRunTest, int, sippyprocessingv1.JobOverallResult, error) {
	failures := 0

	gcsJobRun := gcs.NewGCSJobRun(pl.bkt, path)
	gcsJobRun.SetGCSJunitPaths(junitPaths)
	suites, err := gcsJobRun.GetCombinedJUnitTestSuites(ctx)
	if err != nil {
		log.Warningf("failed to get junit test suites: %s", err.Error())
		return []*models.ProwJobRunTest{}, 0, "", err
	}
	testCases := make(map[string]*models.ProwJobRunTest)
	for _, suite := range suites.Suites {
		suiteID := pl.findSuite(suite.Name)
		if suiteID == nil {
			log.Infof("skipping suite %q as it's not listed for import", suite.Name)
			continue
		}

		pl.extractTestCases(suite, suiteID, testCases)
	}

	syntheticSuite, jobResult := testconversion.ConvertProwJobRunToSyntheticTests(*pj, testCases, pl.syntheticTestManager)

	suiteID := pl.findSuite(syntheticSuite.Name)
	if suiteID == nil {
		// this shouldn't happen but if it does we want to know
		panic("synthetic suite is missing from the database")
	}
	pl.extractTestCases(syntheticSuite, suiteID, testCases)
	log.Infof("synthetic suite had %d tests", syntheticSuite.NumTests)

	results := make([]*models.ProwJobRunTest, 0)
	for k := range testCases {
		if testidentification.IsIgnoredTest(k) {
			continue
		}

		testCases[k].ProwJobRunID = id
		results = append(results, testCases[k])
		if testCases[k].Status == 12 {
			failures++
		}
	}

	return results, failures, jobResult, nil
}

func (pl *ProwLoader) extractTestCases(suite *junit.TestSuite, suiteID *uint, testCases map[string]*models.ProwJobRunTest) {
	testOutputMetadataExtractor := TestFailureMetadataExtractor{}

	for _, tc := range suite.TestCases {
		status := sippyprocessingv1.TestStatusFailure
		var failureOutput *models.ProwJobRunTestOutput
		if tc.SkipMessage != nil {
			continue
		} else if tc.FailureOutput == nil {
			status = sippyprocessingv1.TestStatusSuccess
		} else {
			failureOutput = &models.ProwJobRunTestOutput{
				Output: tc.FailureOutput.Output,
			}
		}

		// Cache key should always have the suite name, so we don't combine
		// a pass and a fail from two different suites to generate a flake.
		testCacheKey := fmt.Sprintf("%s.%s", suite.Name, tc.Name)

		if failureOutput != nil {
			// Check if this test is configured to extract metadata from it's output, and if so, create it
			// in the db.
			extractedMetadata := testOutputMetadataExtractor.ExtractMetadata(tc.Name, failureOutput.Output)
			if len(extractedMetadata) > 0 {
				failureOutput.Metadata = make([]models.ProwJobRunTestOutputMetadata, 0, len(extractedMetadata))
				for _, m := range extractedMetadata {
					jsonb := pgtype.JSONB{}
					if err := jsonb.Set(m); err != nil {
						log.WithError(err).Error("error setting jsonb value with extracted test metadata")
					}
					failureOutput.Metadata = append(failureOutput.Metadata, models.ProwJobRunTestOutputMetadata{
						Metadata: jsonb,
					})
				}
			}
		}

		if existing, ok := testCases[testCacheKey]; !ok {
			testID, err := pl.findOrAddTest(tc.Name)
			if err != nil {
				log.WithError(err).Warningf("could not find or create test %q", tc.Name)
				continue
			}

			testCases[testCacheKey] = &models.ProwJobRunTest{
				TestID:               testID,
				SuiteID:              suiteID,
				Status:               int(status),
				Duration:             tc.Duration,
				ProwJobRunTestOutput: failureOutput,
			}
		} else if (existing.Status == int(sippyprocessingv1.TestStatusFailure) && status == sippyprocessingv1.TestStatusSuccess) ||
			(existing.Status == int(sippyprocessingv1.TestStatusSuccess) && status == sippyprocessingv1.TestStatusFailure) {
			// One pass among failures makes this a flake
			existing.Status = int(sippyprocessingv1.TestStatusFlake)
			if existing.ProwJobRunTestOutput == nil {
				existing.ProwJobRunTestOutput = failureOutput
			}
		}
	}

	for _, c := range suite.Children {
		pl.extractTestCases(c, suiteID, testCases)
	}
}
