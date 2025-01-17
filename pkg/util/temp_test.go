package util

import (
	"fmt"
	"testing"

	"gonum.org/v1/gonum/stat/distuv"
)

// BayesianCalculation calculates the probabilities of the flake vs regression hypotheses.
func BayesianCalculation(priorPasses, priorFailures, successes, failures int) (float64, float64) {

	// Historical data (prior): passes + a smoothing factor
	// Rather than adding 1 for Laplace smoothing, we add 0.5 to increase the
	// weight of a failure in the PR a bit, rather than overly weighting historical.
	smoothing := 1.0
	alphaPrior := float64(priorPasses) + smoothing
	betaPrior := float64(priorFailures) + smoothing

	// New evidence weighting factor:
	newEvidenceWeight := 1.5

	// Update prior with new evidence
	alphaPosterior := alphaPrior + float64(successes)*newEvidenceWeight
	betaPosterior := betaPrior + float64(failures)*newEvidenceWeight

	// Define thresholds for pass rate drop (e.g., below 95% considered regression)
	threshold := 0.95

	// Use the Beta distribution from Gonum
	betaDist := distuv.Beta{Alpha: alphaPosterior, Beta: betaPosterior}

	// Calculate the probability the pass rate is below the threshold (regression hypothesis)
	probRegression := betaDist.CDF(threshold)

	// Calculate the probability the pass rate is above the threshold (flake hypothesis)
	probFlake := 1.0 - probRegression

	return probFlake, probRegression
}

func Test_Bayes(t *testing.T) {

	// Historical pass rate data:
	priorPasses, priorFailures := 98, 2

	// New evidence from the PR: 1 pass, 2 failures
	successes := 1
	failures := 2

	// Calculate probabilities
	probFlake, probRegression := BayesianCalculation(priorPasses, priorFailures, successes, failures)

	// Display the results
	fmt.Printf("Probability of flake: %.4f\n", probFlake)
	fmt.Printf("Probability of regression: %.4f\n", probRegression)
}
