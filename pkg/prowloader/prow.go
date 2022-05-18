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

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	"github.com/openshift/sippy/pkg/testidentification"
)

// FIXME(stbenjam): Make this configurable so we can use kube or openshift prow
const prowURL = "https://prow.ci.openshift.org/prowjobs.js?var=allBuilds&omit=annotations,labels,decoration_config,pod_spec"

type ProwLoader struct {
	dbc                 *db.DB
	bkt                 *storage.BucketHandle
	bktName             string
	prowJobCache        map[string]*models.ProwJob
	prowJobRunCache     map[uint]bool
	prowJobRunTestCache map[string]uint
	variantManager      testidentification.VariantManager
	suiteCache          map[string]uint
}

func New(dbc *db.DB, gcsClient *storage.Client, gcsBucket string, variantManager testidentification.VariantManager) *ProwLoader {
	bkt := gcsClient.Bucket(gcsBucket)

	return &ProwLoader{
		dbc:                 dbc,
		bkt:                 bkt,
		bktName:             gcsBucket,
		prowJobRunCache:     loadProwJobRunCache(dbc),
		prowJobCache:        loadProwJobCache(dbc),
		prowJobRunTestCache: make(map[string]uint),
		suiteCache:          make(map[string]uint),
		variantManager:      variantManager,
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

// Cache the IDs of all known ProwJobRuns for this job. Will be used to skip job run and test
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
	allowedJobRegex := []*regexp.Regexp{
		regexp.MustCompile(`pull-ci-openshift-.*-(master|main)-e2e-.*`), // For now let's just get master/main presubmits in the openshift org
	}

	jobsJSON, err := fetchJobsJSON()
	if err != nil {
		return err
	}
	prowJobs, err := jobsJSONToProwJobs(jobsJSON)
	if err != nil {
		return err
	}

	for _, pj := range prowJobs {
		for _, re := range allowedJobRegex {
			if re.MatchString(pj.Spec.Job) {
				err := pl.prowJobToJobRun(pj)
				if err != nil {
					return err
				}
				break
			}
		}
	}

	return nil
}

func fetchJobsJSON() ([]byte, error) {
	resp, err := http.Get(prowURL)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(resp.Body)
}

func jobsJSONToProwJobs(jobJSON []byte) ([]prow.ProwJob, error) {
	results := make(map[string][]prow.ProwJob)
	// The first 16 bytes are `var allBuilds =`, and then the rest is parseable JSON except for the final character (;).
	if err := json.Unmarshal(jobJSON[16:len(jobJSON)-1], &results); err != nil {
		return nil, err
	}
	return results["items"], nil
}

func (pl *ProwLoader) prowJobToJobRun(pj prow.ProwJob) error {
	releaseRegex := regexp.MustCompile("pull-ci-.*([0-9]+.[0-9]+)-.*")
	matches := releaseRegex.FindStringSubmatch(pj.Spec.Job)
	release := "main"
	if len(matches) > 0 {
		release = matches[1]
	}

	// FIXME(stbenjam): calculate job synthetic tests
	var result sippyprocessingv1.JobOverallResult
	switch pj.Status.State {
	case prow.PendingState:
		// Skip for now, only store runs in a terminal state
		return nil
	case prow.SuccessState:
		result = sippyprocessingv1.JobSucceeded
	case prow.AbortedState:
		result = sippyprocessingv1.JobNoResults
	default:
		result = sippyprocessingv1.JobTestFailure
	}

	id, err := strconv.ParseInt(pj.Status.BuildID, 0, 64)
	if err != nil {
		return nil
	}

	if _, ok := pl.prowJobCache[pj.Spec.Job]; !ok {
		dbProwJob := models.ProwJob{
			Name:     pj.Spec.Job,
			Release:  release,
			Variants: pl.variantManager.IdentifyVariants(pj.Spec.Job),
		}
		err := pl.dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&dbProwJob).Error
		if err != nil {
			return errors.Wrapf(err, "error loading prow job into db: %s", pj.Spec.Job)
		}
		pl.prowJobCache[pj.Spec.Job] = &dbProwJob
	} else {
		// Ensure the job is up to date, especially for variants.
		dbProwJob := pl.prowJobCache[pj.Spec.Job]
		newVariants := pl.variantManager.IdentifyVariants(pj.Spec.Job)
		if !reflect.DeepEqual(newVariants, dbProwJob.Variants) {
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
			tests, failures, err := pl.prowJobRunTestsFromGCS(parts[1][1:])
			if err != nil {
				return err
			}

			pl.dbc.DB.Save(&models.ProwJobRun{
				ProwJob:       *pl.prowJobCache[pj.Spec.Job],
				ProwJobID:     uint(id),
				URL:           pj.Status.URL,
				Timestamp:     pj.Status.StartTime,
				OverallResult: result,
				Tests:         tests,
				TestFailures:  failures,
				Succeeded:     result == sippyprocessingv1.JobSucceeded,
			})
		}
	}

	return nil
}

func (pl *ProwLoader) findOrAddTest(name string) uint {
	if id, ok := pl.prowJobRunTestCache[name]; !ok {
		test := &models.Test{}
		pl.dbc.DB.Where("name = ?", name).Find(&test)
		if test.ID == 0 {
			test.Name = name
			pl.dbc.DB.Save(test)
		}
		pl.prowJobRunTestCache[name] = test.ID
		return test.ID
	} else {
		return id
	}
}

func (pl *ProwLoader) findOrAddSuite(name string) *uint {
	if name == "" {
		return nil
	}

	if id, ok := pl.suiteCache[name]; !ok {
		suite := &models.Suite{}
		pl.dbc.DB.Where("name = ?", name).Find(&suite)
		if suite.ID == 0 {
			suite.Name = name
			pl.dbc.DB.Save(suite)
		}
		id = suite.ID
		pl.suiteCache[name] = id
		return &id
	} else {
		return &id
	}
}

func (pl *ProwLoader) prowJobRunTestsFromGCS(path string) ([]models.ProwJobRunTest, int, error) {
	failures := 0

	gcsJobRun := gcs.NewGCSJobRun(pl.bkt, path)
	suites, err := gcsJobRun.GetCombinedJUnitTestSuites(context.TODO())
	if err != nil {
		log.Warningf("failed to get junit test suites: %s", err.Error())
		return []models.ProwJobRunTest{}, 0, nil
	}
	testCases := make(map[string]*models.ProwJobRunTest)
	for _, suite := range suites.Suites {
		pl.extractTestCases(suite, testCases)
	}

	results := make([]models.ProwJobRunTest, 0)
	for _, v := range testCases {
		results = append(results, *v)
		if v.Status == 12 {
			failures++
		}
	}

	return results, failures, nil
}

func (pl *ProwLoader) extractTestCases(suite *junit.TestSuite, testCases map[string]*models.ProwJobRunTest) {
	for _, tc := range suite.TestCases {
		status := 12
		if tc.FailureOutput == nil {
			status = 1
		}

		key := fmt.Sprintf("%s.%s", suite.Name, tc.Name)
		if existing, ok := testCases[key]; !ok {
			testCases[key] = &models.ProwJobRunTest{
				TestID:   pl.findOrAddTest(tc.Name),
				SuiteID:  pl.findOrAddSuite(suite.Name),
				Status:   status,
				Duration: tc.Duration,
			}
		} else {
			existing.Status += status
		}
	}

	for _, c := range suite.Children {
		pl.extractTestCases(c, testCases)
	}
}
