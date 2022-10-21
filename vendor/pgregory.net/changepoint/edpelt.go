// Copyright 2020 Gregory Petrosyan <gregory.petrosyan@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package changepoint

import (
	"math"
	"sort"
)

// NonParametric returns indexes of elements that split data into
// "statistically homogeneous" segments. NonParametric supports
// nonparametric distributions and has O(N*log(N)) algorithmic complexity.
// NonParametric uses ED-PELT algorithm for changepoint detection.
//
// The implementation is based on the following papers:
//
//   [Haynes2017] Kaylea Haynes, Paul Fearnhead, and Idris A. Eckley.
//   "A computationally efficient nonparametric approach for changepoint detection."
//   Statistics and Computing 27, no. 5 (2017): 1293-1305.
//   https://doi.org/10.1007/s11222-016-9687-5
//
//   [Killick2012] Rebecca Killick, Paul Fearnhead, and Idris A. Eckley.
//   "Optimal detection of changepoints with a linear computational cost."
//   Journal of the American Statistical Association 107, no. 500 (2012): 1590-1598.
//   https://arxiv.org/pdf/1101.1438.pdf
func NonParametric(data []float64, minSegment int) []int {
	if minSegment < 1 {
		panic("minSegment must be positive")
	}

	n := len(data)
	if n <= 2 || n/2 < minSegment {
		return nil
	}

	// The penalty which we add to the final cost for each additional changepoint
	// Here we use the Modified Bayesian Information Criterion
	penalty := 3 * math.Log(float64(n))

	// `k` is the number of quantiles that we use to approximate an integral during the segment cost evaluation
	// We use `k=Ceiling(4*log(n))` as suggested in the Section 4.3 "Choice of K in ED-PELT" in [Haynes2017]
	// `k` can't be greater than `n`, so we should always use the `Min` function here (important for n <= 8)
	k := int(math.Min(float64(n), math.Ceil(4*math.Log(float64(n)))))

	// We should precalculate sums for empirical CDF, it will allow fast evaluating of the segment cost
	partialSums := edPartialSums(data, k)

	cost := func(tau1 int, tau2 int) float64 {
		return edCost(n, k, partialSums, tau1, tau2)
	}

	return pelt(data, minSegment, cost, penalty)
}

// Partial sums for empirical CDF (formula (2.1) from Section 2.1 "Model" in [Haynes2017])
//
//   partialSums'[i, tau] = (count(data[j] < t) * 2 + count(data[j] == t) * 1) for j=0..tau-1
//   where t is the i-th quantile value (see Section 3.1 "Discrete approximation" in [Haynes2017] for details)
//
// In order to get better performance, we present
// a two-dimensional array partialSums'[k, n + 1] as a single-dimensional array partialSums[k * (n + 1)].
// We assume that partialSums'[i, tau] = partialSums[i * (n + 1) + tau].
//
// - We use doubled sum values in order to use []int instead of []float64 (it provides noticeable
//   performance boost). Thus, multipliers for count(data[j] < t) and count(data[j] == t) are
//   2 and 1 instead of 1 and 0.5 from the [Haynes2017].
// - Note that these quantiles are not uniformly distributed: tails of the data distribution contain more
//   quantile values than the center of the distribution
func edPartialSums(data []float64, k int) []int32 {
	n := len(data)
	partialSums := make([]int32, k*(n+1))
	sortedData := append([]float64(nil), data...)
	sort.Float64s(sortedData)

	offset := 0
	for i := 0; i < k; i++ {
		z := -1 + (2*float64(i)+1)/float64(k)       // Values from (-1+1/k) to (1-1/k) with step = 2/k
		p := 1 / (1 + math.Pow(2*float64(n)-1, -z)) // Values from 0.0 to 1.0
		t := sortedData[int(p*float64(n-1))]        // Quantile value, formula (2.1) in [Haynes2017]

		for j, val := range data {
			delta := int32(0)
			if val < t {
				delta = 2 // We use doubled value (2) instead of original 1.0
			} else if val == t {
				delta = 1 // We use doubled value (1) instead of original 0.5
			}

			partialSums[offset+j+1] = partialSums[offset+j] + delta
		}

		offset += n + 1
	}

	return partialSums
}

func edCost(n int, k int, partialSums []int32, tau1 int, tau2 int) float64 {
	tauDiff2i := int32(2 * (tau2 - tau1))
	tauDiff2f := float64(2 * (tau2 - tau1))
	tauDiff2fLog := math.Log(tauDiff2f)

	sum := 0.0
	maxOffset := k * (n + 1)
	for offset := 0; offset < maxOffset; offset += n + 1 {
		// actualSum is (count(data[j] < t) * 2 + count(data[j] == t) * 1) for j=tau1..tau2-1
		actualSum := partialSums[offset+tau2] - partialSums[offset+tau1] // partialSums'[i, tau2] - partialSums'[i, tau1]
		if actualSum == 0 || actualSum == tauDiff2i {
			continue // We skip these two cases (correspond to fit = 0 or fit = 1) because of invalid math.Log values
		}

		// Empirical CDF F_i(t) (Section 2.1 "Model" in [Haynes2017]):
		//   fit = actualSum / (2 * (tau2 - tau1))
		// Segment cost L_np (Section 2.2 "Nonparametric maximum likelihood" in [Haynes2017])
		//   lnp = (tau2 - tau1) * (fit*log(fit) + (1-fit)*log(1-fit))
		// In the end, we multiply sum by 2 (Section 3.1 "Discrete approximation" in [Haynes2017]):
		//   return 2 * c / k * sum
		//
		// Substituting fit into lnp, and transforming log(x/y) into log(x) - log(y),
		// we can get rid of extra multiplications and divisions:

		actualSumF := float64(actualSum)
		actualSumFSub := tauDiff2f - actualSumF

		sum += actualSumF * (math.Log(actualSumF) - tauDiff2fLog)
		sum += actualSumFSub * (math.Log(actualSumFSub) - tauDiff2fLog)
	}

	// Constant from Lemma 3.1 in [Haynes2017]
	c := -math.Log(2*float64(n) - 1)

	return c / float64(k) * sum
}
