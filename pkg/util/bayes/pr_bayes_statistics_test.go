package bayes

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat/distuv"
)

// BayesianSafetyCheck determines if a PR is safe to merge based on historical data,
// other PR results, and environment-specific issues.
func BayesianSafetyCheck(
	historicalPasses, historicalFailures int,                  // simple historical data over the past x weeks
	otherPrPasses, otherPrFailures int, otherPrWeight float64, // recent data we'll weight more heavily to catch on-going incidents
	prPasses, prFailures int,                                  // results from this PR
	thresholdDrop float64,                                     // returns our confidence the test pass rate has dropped more than this amount
) (float64, float64) {

	// Laplace smoothing for historical data
	smoothing := 1.0
	alphaHistorical := float64(historicalPasses) + smoothing
	betaHistorical := float64(historicalFailures) + smoothing

	// Weighted prior contributions
	alphaOtherPr := float64(otherPrPasses)*otherPrWeight + smoothing
	betaOtherPr := float64(otherPrFailures)*otherPrWeight + smoothing

	// Combine priors
	alphaCombined := alphaHistorical + alphaOtherPr
	betaCombined := betaHistorical + betaOtherPr

	// New evidence weighting factor.
	// This is tricky, if we have say 1000 runs in the historical data, no PR can generate enough information
	// to possibly have the model think it could be a regression. We have to weight our PR samples to model
	// our intuition. Because it can depend on the amount of historical data, we do this dynamically based on
	// how much data we're up against.
	newEvidenceWeight := float64(historicalPasses+historicalFailures+otherPrFailures+otherPrFailures) / 20.0
	if newEvidenceWeight < 1.0 {
		newEvidenceWeight = 1.0 // Ensure a minimum weight
	}

	// Adjust combined prior with PR results
	alphaPosterior := alphaCombined + float64(prPasses)*newEvidenceWeight
	betaPosterior := betaCombined + float64(prFailures)*newEvidenceWeight

	// Define threshold for pass rate drop
	historicalRate := float64(historicalPasses) / float64(historicalPasses+historicalFailures)
	threshold := historicalRate - thresholdDrop

	// Beta distribution for posterior
	betaDist := distuv.Beta{Alpha: alphaPosterior, Beta: betaPosterior}

	// Calculate probabilities
	probRegression := betaDist.CDF(threshold)
	probSafe := 1.0 - probRegression

	log.Infof("Historical %d/%d, Recent Jobs %d/%d, This PR: %d/%d failures = Probability regression: %.3f, Probability safe: %.3f",
		historicalPasses, historicalFailures+historicalPasses,
		otherPrPasses, otherPrFailures+otherPrPasses,
		prPasses, prFailures+prPasses, probRegression, probSafe)

	return probSafe, probRegression
}

// Example usage
func Test_PRSafetyCheck(t *testing.T) {
	BayesianSafetyCheck(
		1000, 10,
		20, 2, 3.0,
		1, 1,
		0.05)
	BayesianSafetyCheck(
		29, 1,
		5, 0, 3.0,
		1, 1,
		0.05)
	BayesianSafetyCheck(
		29, 1,
		20, 0, 3.0,
		9, 1,
		0.05)

	// Now lets model strong high pass rate historical data, but this test is failing outside our PR in recent runs,
	// be that other PRs or periodics:
	BayesianSafetyCheck(
		1000, 10,
		20, 5, 1000,
		1, 1,
		0.05)
}
