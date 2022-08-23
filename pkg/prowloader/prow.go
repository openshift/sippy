package prowloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
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
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	"github.com/openshift/sippy/pkg/prowloader/github"
	"github.com/openshift/sippy/pkg/prowloader/testconversion"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
)

type ProwLoader struct {
	dbc                  *db.DB
	bkt                  *storage.BucketHandle
	bktName              string
	githubClient         *github.Client
	prowJobCache         map[string]*models.ProwJob
	prowJobRunCache      map[uint]bool
	prowJobRunTestCache  map[string]uint
	variantManager       testidentification.VariantManager
	suiteCache           map[string]*uint
	syntheticTestManager synthetictests.SyntheticTestManager
	releases             []string
	config               *v1config.SippyConfig
}

func New(dbc *db.DB, gcsClient *storage.Client, gcsBucket string, githubClient *github.Client, variantManager testidentification.VariantManager,
	syntheticTestManager synthetictests.SyntheticTestManager, releases []string, config *v1config.SippyConfig) *ProwLoader {
	bkt := gcsClient.Bucket(gcsBucket)

	return &ProwLoader{
		dbc:                  dbc,
		bkt:                  bkt,
		bktName:              gcsBucket,
		githubClient:         githubClient,
		prowJobRunCache:      loadProwJobRunCache(dbc),
		prowJobCache:         loadProwJobCache(dbc),
		prowJobRunTestCache:  make(map[string]uint),
		suiteCache:           make(map[string]*uint),
		syntheticTestManager: syntheticTestManager,
		variantManager:       variantManager,
		releases:             releases,
		config:               config,
	}
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

func (pl *ProwLoader) LoadProwJobsToDB() error {
	// Update unmerged PR statuses in case any have merged
	if err := pl.syncPRStatus(); err != nil {
		return err
	}

	// Fetch/update job data
	jobsJSON, err := fetchJobsJSON(pl.config.Prow.URL)
	if err != nil {
		return err
	}
	prowJobs, err := jobsJSONToProwJobs(jobsJSON)
	if err != nil {
		return err
	}

	for _, pj := range prowJobs {
		for _, release := range pl.releases {
			cfg, ok := pl.config.Releases[release]
			if !ok {
				log.Warningf("configuration not found for release %q", release)
				continue
			}

			if val, ok := cfg.Jobs[pj.Spec.Job]; val && ok {
				if err := pl.prowJobToJobRun(pj, release); err != nil {
					return err
				}
				break
			}

			for _, expr := range cfg.Regexp {
				re, err := regexp.Compile(expr)
				if err != nil {
					log.WithError(err).Errorf("invalid regex in configuration")
					return fmt.Errorf("invalid regex in configuration %w", err)
				}

				if re.MatchString(pj.Spec.Job) {
					if err := pl.prowJobToJobRun(pj, release); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func (pl *ProwLoader) syncPRStatus() error {
	if pl.githubClient == nil {
		log.Infof("No GitHub client, skipping PR sync")
		return nil
	}

	pulls := make([]models.ProwPullRequest, 0)
	pl.dbc.DB.Table("prow_pull_requests").Where("merged_at IS NULL").Scan(&pulls)
	for _, pr := range pulls {
		mergedAt, err := pl.githubClient.GetPRMerged(pr.Org, pr.Repo, pr.Number, pr.SHA)
		if err != nil {
			log.WithError(err).Warningf("could not fetch pull request status from GitHub; org=%q repo=%q number=%q sha=%q", pr.Org, pr.Repo, pr.Number, pr.SHA)
			return err
		}
		pr.MergedAt = mergedAt
		if res := pl.dbc.DB.Save(pr); res.Error != nil {
			log.WithError(res.Error).Errorf("unexpected error updating pull request %s (%s)", pr.Link, pr.SHA)
			return res.Error
		}
	}

	return nil
}

func fetchJobsJSON(prowURL string) ([]byte, error) {
	resp, err := http.Get(prowURL) // #nosec G107
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(resp.Body)
}

func jobsJSONToProwJobs(jobJSON []byte) ([]prow.ProwJob, error) {
	results := make(map[string][]prow.ProwJob)
	if err := json.Unmarshal(jobJSON, &results); err != nil {
		return nil, err
	}
	return results["items"], nil
}

func (pl *ProwLoader) prowJobToJobRun(pj prow.ProwJob, release string) error {
	if pj.Status.State == prow.PendingState || pj.Status.State == prow.TriggeredState {
		// Skip for now, only store runs in a terminal state
		return nil
	}

	id, err := strconv.ParseUint(pj.Status.BuildID, 0, 64)
	if err != nil {
		return nil
	}

	dbProwJob, foundProwJob := pl.prowJobCache[pj.Spec.Job]
	if !foundProwJob {
		dbProwJob = &models.ProwJob{
			Name:     pj.Spec.Job,
			Kind:     models.ProwKind(pj.Spec.Type),
			Release:  release,
			Variants: pl.variantManager.IdentifyVariants(pj.Spec.Job),
		}
		err := pl.dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(dbProwJob).Error
		if err != nil {
			return errors.Wrapf(err, "error loading prow job into db: %s", pj.Spec.Job)
		}
		pl.prowJobCache[pj.Spec.Job] = dbProwJob
	} else {
		newVariants := pl.variantManager.IdentifyVariants(pj.Spec.Job)
		if !reflect.DeepEqual(newVariants, dbProwJob.Variants) || dbProwJob.Kind != models.ProwKind(pj.Spec.Type) {
			dbProwJob.Kind = models.ProwKind(pj.Spec.Type)
			dbProwJob.Variants = newVariants
			pl.dbc.DB.Save(&dbProwJob)
		}
	}

	if _, ok := pl.prowJobRunCache[uint(id)]; !ok {
		pjURL, err := url.Parse(pj.Status.URL)
		if err != nil {
			return err
		}

		parts := strings.Split(pjURL.Path, pl.bktName)
		if len(parts) == 2 {
			tests, failures, overallResult, err := pl.prowJobRunTestsFromGCS(pj, uint(id), parts[1][1:])
			if err != nil {
				return err
			}
			pulls := pl.findOrAddPullRequests(pj.Spec.Refs)

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

			err = pl.dbc.DB.CreateInBatches(tests, 1000).Error
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (pl *ProwLoader) findOrAddPullRequests(refs *prow.Refs) []models.ProwPullRequest {
	if refs == nil || pl.githubClient == nil {
		return nil
	}
	pulls := make([]models.ProwPullRequest, 0)

	for _, pr := range refs.Pulls {
		if pr.Link == "" {
			continue
		}

		mergedAt, err := pl.githubClient.GetPRMerged(refs.Org, refs.Repo, pr.Number, pr.SHA)
		if err != nil {
			log.WithError(err).Warningf("could not fetch pull request status from GitHub; org=%q repo=%q number=%q sha=%q", refs.Org, refs.Repo, pr.Number, pr.SHA)
		}

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

func (pl *ProwLoader) prowJobRunTestsFromGCS(pj prow.ProwJob, id uint, path string) ([]*models.ProwJobRunTest, int, sippyprocessingv1.JobOverallResult, error) {
	failures := 0

	gcsJobRun := gcs.NewGCSJobRun(pl.bkt, path)
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

		testNameWithKnownSuite := tc.Name
		suiteID := pl.findSuite(suite.Name)
		if suiteID == nil {
			testNameWithKnownSuite = fmt.Sprintf("%s.%s", suite.Name, tc.Name)
		}

		if existing, ok := testCases[testNameWithKnownSuite]; !ok {
			testCases[testNameWithKnownSuite] = &models.ProwJobRunTest{
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
