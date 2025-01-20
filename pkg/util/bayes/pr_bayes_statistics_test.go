package bayes

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat/distuv"
)

// BayesianSafetyCheck determines if a PR is safe to merge based on historical data,
// other PR results, and environment-specific issues.
func BayesianSafetyCheck(
	historicalPasses, historicalFailures int,
	otherPrPasses, otherPrFailures int, otherPrWeight float64,
	envPasses, envFailures int, envWeight float64,
	prPasses, prFailures int, prWeight float64,
	thresholdDrop float64,
) (float64, float64) {

	// Laplace smoothing for historical data
	smoothing := 1.0
	alphaHistorical := float64(historicalPasses) + smoothing
	betaHistorical := float64(historicalFailures) + smoothing

	// Weighted prior contributions
	alphaOtherPr := float64(otherPrPasses)*otherPrWeight + smoothing
	betaOtherPr := float64(otherPrFailures)*otherPrWeight + smoothing

	alphaEnv := float64(envPasses)*envWeight + smoothing
	betaEnv := float64(envFailures)*envWeight + smoothing

	// Combine priors
	alphaCombined := alphaHistorical + alphaOtherPr + alphaEnv
	betaCombined := betaHistorical + betaOtherPr + betaEnv

	// Adjust combined prior with PR results
	alphaPosterior := alphaCombined + float64(prPasses)*prWeight
	betaPosterior := betaCombined + float64(prFailures)*prWeight

	// Define threshold for pass rate drop
	historicalRate := float64(historicalPasses) / float64(historicalPasses+historicalFailures)
	threshold := historicalRate - thresholdDrop

	// Beta distribution for posterior
	betaDist := distuv.Beta{Alpha: alphaPosterior, Beta: betaPosterior}

	// Calculate probabilities
	probRegression := betaDist.CDF(threshold)
	probSafe := 1.0 - probRegression

	log.Printf("PR Results: %d passes, %d failures | Probability regression: %.3f, Probability safe: %.3f",
		prPasses, prFailures, probRegression, probSafe)

	return probSafe, probRegression
}

// Example usage
func Test_PRSafetyCheck(t *testing.T) {
	historicalPasses := 1000
	historicalFailures := 10
	otherPrPasses := 200
	otherPrFailures := 5
	envPasses := 150
	envFailures := 10
	prPasses := 8
	prFailures := 2

	// Example weights
	otherPrWeight := 3.0
	envWeight := 2.0
	prWeight := 5.0

	// Allowable drop in pass rate
	thresholdDrop := 0.05

	probSafe, probRegression := BayesianSafetyCheck(
		historicalPasses, historicalFailures,
		otherPrPasses, otherPrFailures, otherPrWeight,
		envPasses, envFailures, envWeight,
		prPasses, prFailures, prWeight,
		thresholdDrop,
	)

	log.Infof("Probability of PR being safe to merge: %.3f", probSafe)
	log.Infof("Probability of regression: %.3f", probRegression)
}
