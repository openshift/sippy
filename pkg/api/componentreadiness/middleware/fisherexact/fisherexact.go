package fisherexact

import (
	"context"
	"fmt"
	"sync"

	"github.com/apache/thrift/lib/go/thrift"
	fischer "github.com/glycerine/golang-fisher-exact"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/analysis"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

type FisherExact struct {
	reqOptions reqopts.RequestOptions
}

func NewFisherExactMiddleware(reqOptions reqopts.RequestOptions) *FisherExact {
	return &FisherExact{reqOptions: reqOptions}
}

func (f *FisherExact) Query(_ context.Context, _ *sync.WaitGroup, _ crtest.JobVariants,
	_, _ chan map[string]crstatus.TestStatus, _ chan error) {
}

func (f *FisherExact) QueryTestDetails(_ context.Context, _ *sync.WaitGroup, _ chan error, _ crtest.JobVariants) {
}

func (f *FisherExact) PreAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (f *FisherExact) PostAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (f *FisherExact) PreTestDetailsAnalysis(_ crtest.KeyWithVariants, _ *crstatus.TestJobRunStatuses) error {
	return nil
}

// Analyze is the catch-all analysis middleware. It always claims the test and performs
// Fisher's exact test to determine regression significance.
func (f *FisherExact) Analyze(_ crtest.Identification, testStats *testdetails.TestComparison) (bool, error) {
	logger := log.WithField("middleware", "FisherExact")
	opts := f.reqOptions.AdvancedOption

	if testStats.RequiredConfidence == 0 {
		testStats.RequiredConfidence = opts.Confidence
	}

	fisherExactVal := 0.0
	testStats.Comparison = crtest.FisherExact

	status := crtest.MissingBasis
	if testStats.SampleStats.Total() == 0 {
		if opts.IgnoreMissing {
			status = crtest.NotSignificant
		} else {
			status = crtest.MissingSample
		}
		testStats.ReportStatus = status
		testStats.FisherExact = thrift.Float64Ptr(0.0)
		testStats.Explanations = append(testStats.Explanations, analysis.ExplanationNoRegression)
		return true, nil
	} else if testStats.BaseStats != nil && testStats.BaseStats.Total() != 0 {
		samplePass := testStats.SampleStats.Passes(opts.FlakeAsFailure)
		basePass := testStats.BaseStats.Passes(opts.FlakeAsFailure)
		basisPassPercentage := float64(basePass) / float64(testStats.BaseStats.Total())
		effectivePityFactor := float64(opts.PityFactor) + testStats.PityAdjustment
		effectiveMinimumFailure := opts.MinimumFailure

		status = crtest.NotSignificant

		samplePassPercentage := float64(samplePass) / float64(testStats.SampleStats.Total())

		if effectiveMinimumFailure != 0 &&
			(testStats.SampleStats.Total()-samplePass) < effectiveMinimumFailure {
			if status <= crtest.SignificantTriagedRegression {
				testStats.Explanations = append(testStats.Explanations,
					fmt.Sprintf("%s regression detected.", crtest.StringForStatus(status)))
			}
			testStats.ReportStatus = status
			testStats.FisherExact = thrift.Float64Ptr(0.0)
			return true, nil
		}
		significant := false
		improved := samplePassPercentage >= basisPassPercentage

		if improved {
			significant, fisherExactVal = fisherExactTest(testStats.RequiredConfidence, testStats.BaseStats.Total()-basePass, basePass, testStats.SampleStats.Total()-samplePass, samplePass)
		} else if basisPassPercentage-samplePassPercentage > effectivePityFactor/100 {
			significant, fisherExactVal = fisherExactTest(testStats.RequiredConfidence, testStats.SampleStats.Total()-samplePass, samplePass, testStats.BaseStats.Total()-basePass, basePass)
		}
		logger.Debugf("computed Fisher info: signifcant: %v, fisherExact: %v", significant, fisherExactVal)
		if significant {
			if improved {
				status = crtest.SignificantImprovement
			} else {
				status = getRegressionStatus(basisPassPercentage, samplePassPercentage)
			}
		}
	}
	logger.Debugf("computed status: %d", int(status))
	testStats.ReportStatus = status
	testStats.FisherExact = thrift.Float64Ptr(fisherExactVal)

	baseRelease := "no basis"
	if testStats.BaseStats != nil {
		baseRelease = testStats.BaseStats.Release
	}
	if testStats.ReportStatus <= crtest.SignificantTriagedRegression {
		logger.Debugf("regression detected against: %s", baseRelease)

		if testStats.ReportStatus <= crtest.SignificantRegression {
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("%s regression detected.", crtest.StringForStatus(testStats.ReportStatus)))
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("Fishers Exact probability of a regression: %.2f%%.", float64(100)-*testStats.FisherExact))
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("Test pass rate dropped from %.2f%% to %.2f%%.",
					testStats.BaseStats.SuccessRate*float64(100),
					testStats.SampleStats.SuccessRate*float64(100)))
		} else {
			testStats.Explanations = append(testStats.Explanations,
				fmt.Sprintf("%s regression detected.", crtest.StringForStatus(testStats.ReportStatus)))
		}
	} else {
		logger.Debugf("NO regression detected against: %s", baseRelease)
	}

	return true, nil
}

func fisherExactTest(confidenceRequired, sampleFailure, sampleSuccess, baseFailure, baseSuccess int) (bool, float64) {
	_, _, r, _ := fischer.FisherExactTest(sampleFailure,
		sampleSuccess,
		baseFailure,
		baseSuccess)
	return r < 1-float64(confidenceRequired)/100, r
}

func getRegressionStatus(basisPassPercentage, samplePassPercentage float64) crtest.Status {
	if (basisPassPercentage - samplePassPercentage) > 0.15 {
		return crtest.ExtremeRegression
	}

	return crtest.SignificantRegression
}
