package linkinjector

import (
	"context"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	log "github.com/sirupsen/logrus"
)

var _ middleware.Middleware = &LinkInjector{}

func NewLinkInjectorMiddleware(reqOptions reqopts.RequestOptions, baseURL string) *LinkInjector {
	return &LinkInjector{
		log:        log.WithField("middleware", "LinkInjector"),
		reqOptions: reqOptions,
		baseURL:    baseURL,
	}
}

// LinkInjector middleware injects HATEOAS-style links into test analysis results.
// It adds a "test_details" link that points to the test details API endpoint with
// appropriate query parameters based on the test data and request options.
type LinkInjector struct {
	log        log.FieldLogger
	reqOptions reqopts.RequestOptions
	baseURL    string
}

func (l *LinkInjector) Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtest.JobVariants, baseStatusCh, sampleStatusCh chan map[string]bq.TestStatus, errCh chan error) {
	// unused
}

func (l *LinkInjector) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtest.JobVariants) {
	// unused
}

func (l *LinkInjector) PreAnalysis(testKey crtest.Identification, testStats *testdetails.TestComparison) error {
	// unused
	return nil
}

// PostAnalysis injects HATEOAS links into test analysis results
func (l *LinkInjector) PostAnalysis(testKey crtest.Identification, testStats *testdetails.TestComparison) error {
	// Early return if status is above FixedRegression (i.e. regression has not yet rolled off)
	if testStats.ReportStatus > crtest.FixedRegression {
		return nil
	}

	// Initialize Links map if it doesn't exist
	if testStats.Links == nil {
		testStats.Links = make(map[string]string)
	}

	// Convert variants map to string slice for GenerateTestDetailsURL
	variants := utils.VariantsMapToStringSlice(testKey.Variants)

	// Determine base release override if one was used
	baseReleaseOverride := ""
	if testStats.BaseStats != nil && testStats.BaseStats.Release != l.reqOptions.BaseRelease.Name {
		baseReleaseOverride = testStats.BaseStats.Release
	}

	// Generate test details URL
	testDetailsURL, err := utils.GenerateTestDetailsURL(
		testKey.TestID,
		l.baseURL,
		l.reqOptions.BaseRelease,
		l.reqOptions.SampleRelease,
		l.reqOptions.AdvancedOption,
		l.reqOptions.VariantOption,
		testKey.Component,
		testKey.Capability,
		variants,
		baseReleaseOverride,
	)
	if err != nil {
		l.log.WithError(err).Warnf("failed to generate test details URL for test %s", testKey.TestID)
		return err
	}

	// Add the test_details link
	testStats.Links["test_details"] = testDetailsURL

	return nil
}

func (l *LinkInjector) PreTestDetailsAnalysis(testKey crtest.KeyWithVariants, status *bq.TestJobRunStatuses) error {
	// unused
	return nil
}
