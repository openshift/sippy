package types

type TestCaseEntry struct {
	TestName  string
	SuiteName string
	Status    int
	Duration  float64
	Output    *string
}
