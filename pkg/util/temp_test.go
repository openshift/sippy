package util

import (
	"fmt"
	"testing"

	"gonum.org/v1/gonum/stat/distuv"
)

// BayesianCalculation calculates the probabilities of the flake vs regression hypotheses.
func BayesianCalculation(alphaPrior, betaPrior float64, successes, failures int) (float64, float64) {
	// Update prior with new evidence
	alphaPosterior := alphaPrior + float64(successes)
	betaPosterior := betaPrior + float64(failures)

	// Define thresholds for pass rate drop (e.g., below 90% considered regression)
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

	// Historical data (prior): 98 passes, 2 failures
	alphaPrior := 98.0 + 1 // Add 1 for non-zero prior (Laplace smoothing)
	betaPrior := 2.0 + 1   // Add 1 for non-zero prior (Laplace smoothing)

	// New evidence from the PR: 1 pass, 2 failures
	successes := 1
	failures := 6

	// Calculate probabilities
	probFlake, probRegression := BayesianCalculation(alphaPrior, betaPrior, successes, failures)

	// Display the results
	fmt.Printf("Probability of flake: %.4f\n", probFlake)
	fmt.Printf("Probability of regression: %.4f\n", probRegression)
}
