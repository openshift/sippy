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
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgtype"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	v1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/github/commenter"
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	"github.com/openshift/sippy/pkg/prowloader/github"
	"github.com/openshift/sippy/pkg/prowloader/testconversion"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

type ProwLoader struct {
	dbc                  *db.DB
	bkt                  *storage.BucketHandle
	bktName              string
	githubClient         *github.Client
	bigQueryClient       *bigquery.Client
	prowJobCache         map[string]*models.ProwJob
	prowJobRunCache      map[uint]bool
	prowJobRunTestCache  map[string]uint
	variantManager       testidentification.VariantManager
	suiteCache           map[string]*uint
	syntheticTestManager synthetictests.SyntheticTestManager
	releases             []string
	config               *v1config.SippyConfig
	ghCommenter          *commenter.GitHubCommenter
}

func New(
	dbc *db.DB,
	gcsClient *storage.Client,
	bigQueryClient *bigquery.Client,
	gcsBucket string,
	githubClient *github.Client,
	variantManager testidentification.VariantManager,
	syntheticTestManager synthetictests.SyntheticTestManager,
	config *v1config.SippyConfig,
	ghCommenter *commenter.GitHubCommenter) *ProwLoader {

	bkt := gcsClient.Bucket(gcsBucket)

	return &ProwLoader{
		dbc:                  dbc,
		bkt:                  bkt,
		bktName:              gcsBucket,
		githubClient:         githubClient,
		bigQueryClient:       bigQueryClient,
		prowJobRunCache:      loadProwJobRunCache(dbc),
		prowJobCache:         loadProwJobCache(dbc),
		prowJobRunTestCache:  make(map[string]uint),
		suiteCache:           make(map[string]*uint),
		syntheticTestManager: syntheticTestManager,
		variantManager:       variantManager,
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

func (pl *ProwLoader) LoadProwJobsToDB() []error {
	errs := []error{}
	// Update unmerged PR statuses in case any have merged
	if err := pl.syncPRStatus(); err != nil {
		errs = append(errs, errors.Wrap(err, "error in syncPRStatus"))
	}

	// Grab the ProwJob definitions from prow or CI bigquery. Note that these are the Kube
	// ProwJob CRDs, not our sippy db model ProwJob.
	var prowJobs []prow.ProwJob
	// Fetch/update job data
	if pl.bigQueryClient != nil {
		var bqErrs []error
		prowJobs, bqErrs = pl.fetchProwJobsFromOpenShiftBigQuery()
		if len(bqErrs) > 0 {
			errs = append(errs, bqErrs...)
		}
	} else {
		jobsJSON, err := fetchJobsJSON(pl.config.Prow.URL)
		if err != nil {
			errs = append(errs, errors.Wrap(err, "error fetching job JSON data from prow"))
			return errs
		}
		prowJobs, err = jobsJSONToProwJobs(jobsJSON)
		if err != nil {
			errs = append(errs, errors.Wrap(err, "error decoding job JSON data from prow"))
			return errs
		}
	}

	var newJobRunsCtr int
	for i, pj := range prowJobs {
		for release, cfg := range pl.config.Releases {
			if val, ok := cfg.Jobs[pj.Spec.Job]; val && ok {
				if err := pl.prowJobToJobRun(pj, release, &newJobRunsCtr, i, len(prowJobs)); err != nil {
					err = errors.Wrapf(err, "error converting prow job to job run: %s", pj.Spec.Job)
					log.WithError(err).Warning("prow import error")
					errs = append(errs, err)
				}
				break
			}

			for _, expr := range cfg.Regexp {
				re, err := regexp.Compile(expr)
				if err != nil {
					err = errors.Wrap(err, "invalid regex in configuration")
					log.WithError(err).Errorf("config regex error")
					errs = append(errs, err)
					continue
				}

				if re.MatchString(pj.Spec.Job) {
					if err := pl.prowJobToJobRun(pj, release, &newJobRunsCtr, i, len(prowJobs)); err != nil {
						err = errors.Wrapf(err, "error converting prow job to job run: %s", pj.Spec.Job)
						log.WithError(err).Warning("prow import error")
						errs = append(errs, err)
					}
				}
			}
		}
	}
	log.WithField("newJobRuns", newJobRunsCtr).Info("finished importing new ProwJobs and ProwJobRuns")

	return errs
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
			return testgridhelpers.URLForJob(dashboard, jobName)
		}
	}
	return &url.URL{}
}

func (pl *ProwLoader) getClusterData(path string, matches []string) models.ClusterData {
	// get the variant cluster data for this job run
	gcsJobRun := gcs.NewGCSJobRun(pl.bkt, path)
	cd := models.ClusterData{}

	// return empty struct to pass along
	match := findMostRecentDateTimeMatch(matches)
	if match == "" {
		return cd
	}

	bytes, err := gcsJobRun.GetContent(context.TODO(), match)
	if err != nil {
		log.WithError(err).Errorf("Failed to get prow job variant data for: %s", match)
	} else if bytes != nil {
		err := json.Unmarshal(bytes, &cd)
		if err != nil {
			log.WithError(err).Errorf("Failed to unmarshal prow cluster data for: %s", match)
		}
	}
	return cd
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

func (pl *ProwLoader) prowJobToJobRun(pj prow.ProwJob, release string, newJobRunsCtr *int, currentIndex, totalJobsToScan int) error {
	if pj.Status.State == prow.PendingState || pj.Status.State == prow.TriggeredState {
		// Skip for now, only store runs in a terminal state
		return nil
	}

	id, err := strconv.ParseUint(pj.Status.BuildID, 0, 64)
	if err != nil {
		return nil
	}

	pjLog := log.WithFields(log.Fields{
		"job":      pj.Spec.Job,
		"buildID":  pj.Status.BuildID,
		"start":    pj.Status.StartTime,
		"progress": fmt.Sprintf("%d/%d", currentIndex, totalJobsToScan),
	})

	// this err validation has moved up
	// and will exit before we save / update the ProwJob
	// now, any concerns?
	pjURL, err := url.Parse(pj.Status.URL)
	if err != nil {
		return err
	}

	parts := strings.Split(pjURL.Path, pl.bktName)
	path := parts[1][1:]

	// find all files here then pass to getClusterData
	// and prowJobRunTestsFromGCS
	// add more regexes if we require more
	// results from scanning for file names
	gcsJobRun := gcs.NewGCSJobRun(pl.bkt, path)
	allMatches := gcsJobRun.FindAllMatches([]*regexp.Regexp{gcs.GetDefaultClusterDataFile(), gcs.GetDefaultJunitFile()})
	var clusterMatches []string
	var junitMatches []string
	if len(allMatches) > 0 {
		clusterMatches = allMatches[0]
		junitMatches = allMatches[1]
	}

	clusterData := pl.getClusterData(path, clusterMatches)

	dbProwJob, foundProwJob := pl.prowJobCache[pj.Spec.Job]
	if !foundProwJob {
		pjLog.Info("creating new ProwJob")
		dbProwJob = &models.ProwJob{
			Name:        pj.Spec.Job,
			Kind:        models.ProwKind(pj.Spec.Type),
			Release:     release,
			Variants:    pl.variantManager.IdentifyVariants(pj.Spec.Job, release, clusterData),
			TestGridURL: pl.generateTestGridURL(release, pj.Spec.Job).String(),
		}
		err := pl.dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(dbProwJob).Error
		if err != nil {
			return errors.Wrapf(err, "error loading prow job into db: %s", pj.Spec.Job)
		}
		pl.prowJobCache[pj.Spec.Job] = dbProwJob
	} else {
		saveDB := false
		newVariants := pl.variantManager.IdentifyVariants(pj.Spec.Job, release, clusterData)
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
			pl.dbc.DB.Save(&dbProwJob)
		}
	}

	if _, ok := pl.prowJobRunCache[uint(id)]; !ok {
		pjLog.Info("creating new ProwJobRun")

		if len(parts) == 2 {
			tests, failures, overallResult, err := pl.prowJobRunTestsFromGCS(pj, uint(id), path, junitMatches)
			if err != nil {
				return err
			}

			pulls := pl.findOrAddPullRequests(pj.Spec.Refs, path)

			var duration time.Duration
			if pj.Status.CompletionTime != nil {
				duration = pj.Status.CompletionTime.Sub(pj.Status.StartTime)
			}

			err = pl.dbc.DB.Create(&models.ProwJobRun{
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
			pl.prowJobRunCache[uint(id)] = true

			err = pl.dbc.DB.CreateInBatches(tests, 1000).Error
			if err != nil {
				return err
			}
			*newJobRunsCtr++
		}
	}

	return nil
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

func (pl *ProwLoader) findOrAddTest(name string) uint {
	if id, ok := pl.prowJobRunTestCache[name]; ok {
		return id
	}

	test := &models.Test{}
	pl.dbc.DB.Where("name = ?", name).Find(&test)
	if test.ID == 0 {
		test.Name = name
		pl.dbc.DB.Save(test)
	}
	pl.prowJobRunTestCache[name] = test.ID
	return test.ID
}

func (pl *ProwLoader) findSuite(name string) *uint {
	if name == "" {
		return nil
	}

	if id, ok := pl.suiteCache[name]; ok {
		return id
	}

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

func (pl *ProwLoader) prowJobRunTestsFromGCS(pj prow.ProwJob, id uint, path string, junitPaths []string) ([]*models.ProwJobRunTest, int, sippyprocessingv1.JobOverallResult, error) {
	failures := 0

	gcsJobRun := gcs.NewGCSJobRun(pl.bkt, path)
	gcsJobRun.SetGCSJunitPaths(junitPaths)
	suites, err := gcsJobRun.GetCombinedJUnitTestSuites(context.TODO())
	if err != nil {
		log.Warningf("failed to get junit test suites: %s", err.Error())
		return []*models.ProwJobRunTest{}, 0, "", err
	}
	testCases := make(map[string]*models.ProwJobRunTest)
	for _, suite := range suites.Suites {
		pl.extractTestCases(suite, testCases)
	}

	syntheticSuite, jobResult := testconversion.ConvertProwJobRunToSyntheticTests(pj, testCases, pl.syntheticTestManager)
	pl.extractTestCases(syntheticSuite, testCases)

	results := make([]*models.ProwJobRunTest, 0)
	for k, v := range testCases {
		if testidentification.IsIgnoredTest(k) {
			continue
		}

		v.ProwJobRunID = id
		results = append(results, v)
		if v.Status == 12 {
			failures++
		}
	}

	return results, failures, jobResult, nil
}

func (pl *ProwLoader) extractTestCases(suite *junit.TestSuite, testCases map[string]*models.ProwJobRunTest) {
	testOutputMetadataExtractor := TestFailureMetadataExtractor{}
	for _, tc := range suite.TestCases {
		status := v1.TestStatusFailure
		var failureOutput *models.ProwJobRunTestOutput
		if tc.SkipMessage != nil {
			continue
		} else if tc.FailureOutput == nil {
			status = v1.TestStatusSuccess
		} else {
			failureOutput = &models.ProwJobRunTestOutput{
				Output: tc.FailureOutput.Output,
			}
		}

		// Cache key should always have the suite name, so we don't combine
		// a pass and a fail from two different suites to generate a flake.
		testCacheKey := fmt.Sprintf("%s.%s", suite.Name, tc.Name)

		// For historical reasons (TestGrid didn't know about suites), Sippy
		// only had a limited set of preconfigured suites that it knew. If we don't
		// know the suite, we prepend the suite name.
		testNameWithKnownSuite := tc.Name
		suiteID := pl.findSuite(suite.Name)
		if suiteID == nil && suite.Name != "" {
			testNameWithKnownSuite = fmt.Sprintf("%s.%s", suite.Name, tc.Name)
		}

		if failureOutput != nil {
			// Check if this test is configured to extract metadata from it's output, and if so, create it
			// in the db.
			extractedMetadata := testOutputMetadataExtractor.ExtractMetadata(testNameWithKnownSuite, failureOutput.Output)
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
			testCases[testCacheKey] = &models.ProwJobRunTest{
				TestID:               pl.findOrAddTest(testNameWithKnownSuite),
				SuiteID:              suiteID,
				Status:               int(status),
				Duration:             tc.Duration,
				ProwJobRunTestOutput: failureOutput,
			}
		} else if (existing.Status == int(v1.TestStatusFailure) && status == v1.TestStatusSuccess) ||
			(existing.Status == int(v1.TestStatusSuccess) && status == v1.TestStatusFailure) {
			// One pass among failures makes this a flake
			existing.Status = int(v1.TestStatusFlake)
			if existing.ProwJobRunTestOutput == nil {
				existing.ProwJobRunTestOutput = failureOutput
			}
		}
	}

	for _, c := range suite.Children {
		pl.extractTestCases(c, testCases)
	}
}
