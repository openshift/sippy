package bayes

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat/distuv"
)

// BayesianCalculation calculates the probabilities of the flake vs regression hypotheses.
func BayesianCalculation(priorPasses, priorFailures, samplePasses, sampleFailures int) (float64, float64) {

	// Historical data (prior): passes + a smoothing factor
	// Rather than adding 1 for Laplace smoothing, we add 0.5 to increase the
	// weight of a failure in the PR a bit, rather than overly weighting historical.
	smoothing := 1.0
	alphaPrior := float64(priorPasses) + smoothing
	betaPrior := float64(priorFailures) + smoothing

	// New evidence weighting factor, dynamic as we want to weight our PR heavily even when given
	// a massive historical sample, as we're suspicious of increased failure rates in our our
	// alpha/beta posterior.
	newEvidenceWeight := float64(priorPasses+priorFailures) / 20.0
	if newEvidenceWeight < 1.0 {
		newEvidenceWeight = 1.0 // Ensure a minimum weight
	}

	// Update prior with new evidence
	alphaPosterior := alphaPrior + float64(samplePasses)*newEvidenceWeight
	betaPosterior := betaPrior + float64(sampleFailures)*newEvidenceWeight

	// Define thresholds for pass rate drop, worse than it was historically:
	threshold := (float64(priorPasses) / float64(priorPasses+priorFailures)) - 0.05

	// Use the Beta distribution from Gonum
	betaDist := distuv.Beta{Alpha: alphaPosterior, Beta: betaPosterior}

	// Calculate the probability the pass rate is below the threshold (regression hypothesis)
	probRegression := betaDist.CDF(threshold)

	// Calculate the probability the pass rate is above the threshold (flake hypothesis)
	probFlake := 1.0 - probRegression

	log.Infof("historical %d pass %d fail, new data %d pass %d fail, probability pass rate is now below %.3f: %.3f",
		priorPasses, priorFailures, samplePasses, sampleFailures, threshold, probRegression)

	return probFlake, probRegression
}

func Test_Bayes(t *testing.T) {

	BayesianCalculation(30, 0, 8, 2)
	BayesianCalculation(30, 1, 9, 1)
	BayesianCalculation(30, 1, 5, 5)
	BayesianCalculation(29, 5, 8, 2)
	BayesianCalculation(2000, 3, 0, 1)
	BayesianCalculation(2000, 3, 1, 2)

}
