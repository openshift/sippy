package bayes

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat/distuv"
)

// BayesianSafetyCheck determines if a PR is safe to merge based on historical data,
// other PR results, and environment-specific issues.
func BayesianSafetyCheck(
	historicalPasses, historicalFailures int, // simple historical data over the past x weeks
	recentPasses, recentFailures int,         // recent data we'll weight more heavily to catch on-going incidents
	prPasses, prFailures int,                 // results from this PR
	thresholdDrop float64,                    // returns our confidence the test pass rate has dropped more than this amount
) (float64, float64) {

	// Laplace smoothing for historical data
	smoothing := 1.0
	alphaHistorical := float64(historicalPasses) + smoothing
	betaHistorical := float64(historicalFailures) + smoothing

	// Calculate recent-to-historical ratio
	recentVolume := float64(recentPasses + recentFailures)
	historicalVolume := float64(historicalPasses + historicalFailures)
	volumeScale := recentVolume / (historicalVolume + 1.0) // Avoid division by zero

	// Apply volume scaling to recent results, we want recent results to be considered far more significant
	// than historical.
	// Increase recent weight when failures are ongoing
	recentWeightBoost := 1.0 + (float64(recentFailures)/(float64(recentPasses+1.0)))*2.0
	dynamicWeight := (1.0 + volumeScale*2.0) * recentWeightBoost

	alphaOtherPr := float64(recentPasses)*dynamicWeight + smoothing
	betaOtherPr := float64(recentFailures)*dynamicWeight + smoothing

	// Combine priors
	alphaCombined := alphaHistorical + alphaOtherPr
	betaCombined := betaHistorical + betaOtherPr

	// New evidence weighting factor.
	// This is tricky, if we have say 1000 runs in the historical data, no PR can generate enough information
	// to possibly have the model think it could be a regression. We have to weight our PR samples to model
	// our intuition. Because it can depend on the amount of historical data, we do this dynamically based on
	// how much data we're up against.
	/*
		newEvidenceWeight := float64(historicalPasses+historicalFailures+recentFailures+recentFailures) / 20.0
		if newEvidenceWeight < 1.0 {
			newEvidenceWeight = 1.0 // Ensure a minimum weight
		}

	*/

	// Dynamically limit PR contribution based on historical and recent data volume
	prWeightLimit := (recentVolume + historicalVolume) / 10.0
	prWeight := float64(prPasses+prFailures) / prWeightLimit
	if prWeight < 1.0 {
		prWeight = 1.0 // Minimum weight for PR evidence
	}

	// Adjust combined prior with PR results
	alphaPosterior := alphaCombined + float64(prPasses)*prWeight
	betaPosterior := betaCombined + float64(prFailures)*prWeight

	log.Infof("alpha historical = %.1f, recent = %.1f, pr = %.1f",
		alphaHistorical,
		alphaOtherPr,
		alphaPosterior)
	log.Infof("beta historical = %.1f, recent = %.1f, pr = %.1f",
		betaHistorical,
		betaOtherPr,
		betaPosterior)
	// Define threshold for pass rate drop
	historicalRate := float64(historicalPasses) / float64(historicalPasses+historicalFailures)
	threshold := historicalRate - thresholdDrop

	// Beta distribution for posterior
	betaDist := distuv.Beta{Alpha: alphaPosterior, Beta: betaPosterior}

	// Calculate probabilities
	probRegression := betaDist.CDF(threshold)
	probSafe := 1.0 - probRegression

	log.Infof("Historical %d/%d, Recent Jobs %d/%d, This PR: %d/%d = Probability regression: %.3f, Probability safe: %.3f",
		historicalPasses, historicalFailures+historicalPasses,
		recentPasses, recentFailures+recentPasses,
		prPasses, prFailures+prPasses, probRegression, probSafe)

	return probSafe, probRegression
}

// Example usage
func Test_PRSafetyCheck(t *testing.T) {
	// passRateDrop is the drop in pass rate we're testing certainty for,
	// i.e. how certain are we the tests pass rate has dropped this percentage if we merge this PR
	passRateDrop := 0.05

	log.Info("Significant historical, limited mixed results in PR, possible on-going issue")
	BayesianSafetyCheck(
		1000, 10,
		20, 7,
		1, 1,
		passRateDrop)

	log.Info("Limited historical, limited mixed results in PR")
	BayesianSafetyCheck(
		29, 1,
		5, 0,
		1, 1,
		passRateDrop)

	log.Info("Limited historical, unlikely regression in PR")
	BayesianSafetyCheck(
		29, 1,
		20, 0,
		9, 1,
		passRateDrop)

	log.Info("Limited historical, obvious regression in PR")
	BayesianSafetyCheck(
		29, 1,
		20, 0,
		5, 10,
		passRateDrop)

	// Now lets model strong high pass rate historical data, but this test is failing outside our PR in recent runs,
	// be that other PRs or periodics:
	log.Info("Stable test, on-going incident outside PR")
	BayesianSafetyCheck(
		1000, 0,
		10, 20,
		1, 2,
		passRateDrop)
}
