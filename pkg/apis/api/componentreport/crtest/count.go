package crtest

// TestCount is a struct representing the counts of test results in BigQuery-land.
type TestCount struct {
	TotalCount   int `json:"total_count" bigquery:"total_count"`
	SuccessCount int `json:"success_count" bigquery:"success_count"`
	FlakeCount   int `json:"flake_count" bigquery:"flake_count"`
}

//nolint:revive
func (tc TestCount) Add(add TestCount) TestCount {
	tc.TotalCount += add.TotalCount
	tc.SuccessCount += add.SuccessCount
	tc.FlakeCount += add.FlakeCount
	return tc
}
func (tc TestCount) Failures() int { // translate to sippy/stats-land
	failure := tc.TotalCount - tc.SuccessCount - tc.FlakeCount
	if failure < 0 { // this shouldn't happen but just as a failsafe...
		failure = 0
	}
	return failure
}
func (tc TestCount) ToTestStats(flakeAsFailure bool) TestDetailsTestStats { // translate to sippy/stats-land
	return NewTestStats(tc.SuccessCount, tc.Failures(), tc.FlakeCount, flakeAsFailure)
}

type TestDetailsTestStats struct {
	SuccessCount int `json:"success_count"`
	FailureCount int `json:"failure_count"`
	FlakeCount   int `json:"flake_count"`
	// calculate from the above with PassRate method:
	SuccessRate float64 `json:"success_rate"`
}

func (tdts TestDetailsTestStats) Total() int {
	return tdts.SuccessCount + tdts.FailureCount + tdts.FlakeCount
}

func (tdts TestDetailsTestStats) Passes(flakesAsFailure bool) int {
	if flakesAsFailure {
		return tdts.SuccessCount
	}
	return tdts.SuccessCount + tdts.FlakeCount
}

func (tdts TestDetailsTestStats) PassRate(flakesAsFailure bool) float64 {
	return CalculatePassRate(tdts.SuccessCount, tdts.FailureCount, tdts.FlakeCount, flakesAsFailure)
}

func (tdts TestDetailsTestStats) Add(add TestDetailsTestStats, flakesAsFailure bool) TestDetailsTestStats {
	return NewTestStats(
		tdts.SuccessCount+add.SuccessCount,
		tdts.FailureCount+add.FailureCount,
		tdts.FlakeCount+add.FlakeCount,
		flakesAsFailure,
	)
}

func (tdts TestDetailsTestStats) AddTestCount(add TestCount, flakesAsFailure bool) TestDetailsTestStats {
	return NewTestStats(
		tdts.SuccessCount+add.SuccessCount,
		tdts.FailureCount+add.Failures(),
		tdts.FlakeCount+add.FlakeCount,
		flakesAsFailure,
	)
}

func (tdts TestDetailsTestStats) FailPassWithFlakes(flakesAsFailure bool) (int, int) {
	if flakesAsFailure {
		return tdts.FailureCount + tdts.FlakeCount, tdts.SuccessCount
	}
	return tdts.FailureCount, tdts.SuccessCount + tdts.FlakeCount
}

func NewTestStats(successCount, failureCount, flakeCount int, flakesAsFailure bool) TestDetailsTestStats {
	return TestDetailsTestStats{
		SuccessCount: successCount,
		FailureCount: failureCount,
		FlakeCount:   flakeCount,
		SuccessRate:  CalculatePassRate(successCount, failureCount, flakeCount, flakesAsFailure),
	}
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
