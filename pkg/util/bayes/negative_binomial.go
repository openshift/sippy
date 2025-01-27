package main

import (
	"fmt"
	"math"
)

// Calculate the posterior parameters
func posteriorParams(alphaPrior, betaPrior, kRecent, nRecent float64) (float64, float64) {
	alphaPosterior := alphaPrior + kRecent
	betaPosterior := betaPrior + nRecent
	return alphaPosterior, betaPosterior
}

// Factorial function
func factorial(n int) float64 {
	if n == 0 {
		return 1
	}
	result := 1.0
	for i := 1; i <= n; i++ {
		result *= float64(i)
	}
	return result
}

// Binomial coefficient function
func binomialCoefficient(n, k int) float64 {
	if k > n {
		return 0
	}
	return factorial(n) / (factorial(k) * factorial(n-k))
}

// Negative Binomial probability calculation
func negativeBinomial(k int, size, prob float64) float64 {
	chooseFactor := binomialCoefficient(k+int(size)-1, k)
	return chooseFactor * math.Pow(prob, size) * math.Pow(1-prob, float64(k))
}

// Calculate the posterior predictive probability
func posteriorPredictive(alphaPosterior, betaPosterior, k, n float64) float64 {
	prob := betaPosterior / (betaPosterior + n)
	return negativeBinomial(int(k), alphaPosterior, prob)
}

// Analyze a scenario
func analyzeScenario(alphaPrior, betaPrior, kRecent, nRecent, kPr, nPr float64) {
	// Step 1: Update posterior parameters
	alphaPosterior, betaPosterior := posteriorParams(alphaPrior, betaPrior, kRecent, nRecent)

	// Step 2: Calculate posterior predictive probability for pull request data
	prob := posteriorPredictive(alphaPosterior, betaPosterior, kPr, nPr)

	// Step 3: Report results
	fmt.Println("--- Results ---")
	fmt.Printf("Posterior Mean Failure Rate: %.4f\n", alphaPosterior/betaPosterior)
	fmt.Printf("Posterior Predictive Probability of %.0f failures in %.0f tests: %.6f\n\n", kPr, nPr, prob)
}

func main() {
	fmt.Println("Significant historical, limited mixed results in PR, possible ongoing issue")
	analyzeScenario(10, 1000, 7, 27, 1, 2)

	fmt.Println("Limited historical, limited mixed results in PR")
	analyzeScenario(1, 30, 0, 5, 1, 2)

	fmt.Println("Limited historical, unlikely regression in PR")
	analyzeScenario(1, 30, 0, 20, 1, 10)

	fmt.Println("Limited historical, obvious regression in PR")
	analyzeScenario(1, 30, 0, 20, 10, 15)

	fmt.Println("Strong high pass rate historical data, but this test is failing outside our PR in recent runs")
	analyzeScenario(0, 1000, 20, 30, 1, 3)

	fmt.Println("Slight Regression found from CR")
	analyzeScenario(0, 418, 8, 106, 3, 10)
}

