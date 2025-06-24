package crtest

// test Count and Stats types represent much the same concept,
// but Count is used as basically a DAO for BigQuery test results and summations,
// while Stats represent test results to sippy users with pass rates built in.
// really, Count belongs in the bq package, but since these types are so closely related,
// and each has methods to translate the other, they need to be together to prevent circular dependencies.

// Count is a struct representing the counts of test results from BigQuery-land.
// initial counts from the DB are always 0 or 1, but these can be aggregated via Add().
type Count struct {
	TotalCount   int `json:"total_count" bigquery:"total_count"`
	SuccessCount int `json:"success_count" bigquery:"success_count"`
	FlakeCount   int `json:"flake_count" bigquery:"flake_count"`
}

//nolint:revive
func (tc Count) Add(add Count) Count {
	tc.TotalCount += add.TotalCount
	tc.SuccessCount += add.SuccessCount
	tc.FlakeCount += add.FlakeCount
	return tc
}
func (tc Count) Failures() int { // translate to sippy/stats-land
	failure := tc.TotalCount - tc.SuccessCount - tc.FlakeCount
	if failure < 0 { // this shouldn't happen but just as a failsafe...
		failure = 0
	}
	return failure
}
func (tc Count) ToTestStats(flakeAsFailure bool) Stats { // translate to sippy/stats-land
	return NewTestStats(tc.SuccessCount, tc.Failures(), tc.FlakeCount, flakeAsFailure)
}

// Stats represents test result counts for sippy viewers; the attributes should be considered read-only,
// created and modified only via methods, which consistently calculate SuccessRate according to
// whether we consider flakes success or not.
type Stats struct {
	SuccessCount int `json:"success_count"`
	FailureCount int `json:"failure_count"`
	FlakeCount   int `json:"flake_count"`
	// calculate from the above with PassRate method:
	SuccessRate float64 `json:"success_rate"`
}

func NewTestStats(successCount, failureCount, flakeCount int, flakesAsFailure bool) Stats {
	return Stats{
		SuccessCount: successCount,
		FailureCount: failureCount,
		FlakeCount:   flakeCount,
		SuccessRate:  CalculatePassRate(successCount, failureCount, flakeCount, flakesAsFailure),
	}
}

func (ts Stats) Total() int {
	return ts.SuccessCount + ts.FailureCount + ts.FlakeCount
}

func (ts Stats) Passes(flakesAsFailure bool) int {
	if flakesAsFailure {
		return ts.SuccessCount
	}
	return ts.SuccessCount + ts.FlakeCount
}

func (ts Stats) PassRate(flakesAsFailure bool) float64 {
	return CalculatePassRate(ts.SuccessCount, ts.FailureCount, ts.FlakeCount, flakesAsFailure)
}

func (ts Stats) Add(add Stats, flakesAsFailure bool) Stats {
	return NewTestStats(
		ts.SuccessCount+add.SuccessCount,
		ts.FailureCount+add.FailureCount,
		ts.FlakeCount+add.FlakeCount,
		flakesAsFailure,
	)
}

func (ts Stats) AddTestCount(add Count, flakesAsFailure bool) Stats {
	return NewTestStats(
		ts.SuccessCount+add.SuccessCount,
		ts.FailureCount+add.Failures(),
		ts.FlakeCount+add.FlakeCount,
		flakesAsFailure,
	)
}

func (ts Stats) FailPassWithFlakes(flakesAsFailure bool) (int, int) {
	if flakesAsFailure {
		return ts.FailureCount + ts.FlakeCount, ts.SuccessCount
	}
	return ts.FailureCount, ts.SuccessCount + ts.FlakeCount
}

func CalculatePassRate(success, failure, flake int, treatFlakeAsFailure bool) float64 {
	total := success + failure + flake
	if total == 0 {
		return 0.0
	}
	if treatFlakeAsFailure {
		return float64(success) / float64(total)
	}
	return float64(success+flake) / float64(total)
}
