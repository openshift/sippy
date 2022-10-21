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

// Package changepoint implements algorithms for changepoint detection.
package changepoint

import "math"

// Calculates the cost of the (tau1; tau2] segment.
// Remember that tau are one-based indexes.
type costFunc func(tau1 int, tau2 int) float64

func pelt(data []float64, minSegment int, cost costFunc, penalty float64) []int {
	n := len(data)

	// We will use dynamic programming to find the best solution; `bestCost` is the cost array.
	// `bestCost[i]` is the cost for subarray `data[0..i-1]`.
	// It's a 1-based array (`data[0]`..`data[n-1]` correspond to `bestCost[1]`..`bestCost[n]`)
	bestCost := make([]float64, n+1)
	bestCost[0] = -penalty
	for curTau := minSegment; curTau < 2*minSegment; curTau++ {
		bestCost[curTau] = cost(0, curTau)
	}

	// `prevChangepointIndex` is an array of references to previous changepoints. If the current segment ends at
	// the position `i`, the previous segment ends at the position `prevChangepointIndex[i]`. It's a 1-based
	// array (`data[0]`..`data[n-1]` correspond to the `prevChangepointIndex[1]`..`prevChangepointIndex[n]`)
	prevChangepointIndex := make([]int, n+1)

	// We use PELT (Pruned Exact Linear Time) approach which means that instead of enumerating all possible previous
	// tau values, we use a whitelist of "good" tau values that can be used in the optimal solution. If we are 100%
	// sure that some of the tau values will not help us to form the optimal solution, such values should be
	// removed. See [Killick2012] for details.
	prevTaus := make([]int, n+1) // The maximum number of the previous tau values is n + 1
	prevTaus[0] = 0
	prevTaus[1] = minSegment
	costForPrevTau := make([]float64, n+1)
	prevTausCount := 2 // The counter of previous tau values. Defines the size of `prevTaus` and `costForPrevTau`.

	// Following the dynamic programming approach, we enumerate all tau positions. For each `curTau`, we pretend
	// that it's the end of the last segment and trying to find the end of the previous segment.
	for curTau := 2 * minSegment; curTau < n+1; curTau++ {
		// For each previous tau, we should calculate the cost of taking this tau as the end of the previous
		// segment. This cost equals the cost for the `prevTau` plus cost of the new segment (from `prevTau`
		// to `curTau`) plus penalty for the new changepoint.
		for i, prevTau := range prevTaus[:prevTausCount] {
			costForPrevTau[i] = bestCost[prevTau] + cost(prevTau, curTau) + penalty
		}

		// Now we should choose the tau that provides the minimum possible cost.
		bestPrevTauIndex, curBestCost := whichMin(costForPrevTau[:prevTausCount])
		bestCost[curTau] = curBestCost
		prevChangepointIndex[curTau] = prevTaus[bestPrevTauIndex]

		// Prune phase: we remove "useless" tau values that will not help to achieve minimum cost in the future
		newPrevTausCount := 0
		for i, prevTauCost := range costForPrevTau[:prevTausCount] {
			if prevTauCost < curBestCost+penalty {
				prevTaus[newPrevTausCount] = prevTaus[i]
				newPrevTausCount++
			}
		}

		// We add a new tau value that is located on the `minSegment` distance from the next `curTau` value
		prevTaus[newPrevTausCount] = curTau - minSegment + 1
		prevTausCount = newPrevTausCount + 1
	}

	// Here we collect the result list of changepoint indexes `changepoints` using `prevChangepointIndex`
	var changepoints []int
	// The index of the end of the last segment is `n`
	for i := prevChangepointIndex[n]; i != 0; i = prevChangepointIndex[i] {
		changepoints = append(changepoints, i) // 1-based index of the end of segment is equal to 0-based index of the beginning of next segment
	}

	// The result changepoints should be sorted in ascending order.
	for l, r := 0, len(changepoints)-1; l < r; l, r = l+1, r-1 {
		changepoints[l], changepoints[r] = changepoints[r], changepoints[l]
	}

	return changepoints
}

func whichMin(data []float64) (int, float64) {
	ix, min := -1, math.Inf(1)

	for i, v := range data {
		if v < min {
			ix, min = i, v
		}
	}

	return ix, min
}
