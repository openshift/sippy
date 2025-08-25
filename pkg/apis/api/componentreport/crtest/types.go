package crtest

import (
	"encoding/json"
	"time"
)

// These are foundational types for tests, built only from basic golang types.
// They are test data building blocks shared by test_details and component reports.

type ColumnID string

type Identification struct {
	RowIdentification
	ColumnIdentification
}

type RowIdentification struct {
	Component  string `json:"component"`
	Capability string `json:"capability,omitempty"`
	TestName   string `json:"test_name,omitempty"`
	TestSuite  string `json:"test_suite,omitempty"`
	TestID     string `json:"test_id,omitempty"`
}

type ColumnIdentification struct {
	Variants map[string]string `json:"variants"`
}

type Status int

// Comparison is the type of comparison done for a test that has been marked red.
type Comparison string

const (
	PassRate    Comparison = "pass_rate"
	FisherExact Comparison = "fisher_exact"
)

const (
	// FailedFixedRegression indicates someone has claimed the bug is fix, but we see failures past the resolution time
	FailedFixedRegression Status = -1000
	// ExtremeRegression shows regression with >15% pass rate change
	ExtremeRegression Status = -500
	// SignificantRegression shows significant regression
	SignificantRegression Status = -400
	// ExtremeTriagedRegression shows an ExtremeRegression that clears when Triaged incidents are factored in
	ExtremeTriagedRegression Status = -300
	// SignificantTriagedRegression shows a SignificantRegression that clears when Triaged incidents are factored in
	SignificantTriagedRegression Status = -200
	// FixedRegression indicates someone has claimed the bug is now fixed, but has not yet rolled off the sample window
	FixedRegression Status = -150
	// MissingSample indicates sample data missing
	MissingSample Status = -100
	// NotSignificant indicates no significant difference
	NotSignificant Status = 0
	// MissingBasis indicates basis data missing
	MissingBasis Status = 100
	// MissingBasisAndSample indicates basis and sample data missing
	MissingBasisAndSample Status = 200
	// SignificantImprovement indicates improved sample rate
	SignificantImprovement Status = 300
)

func StringForStatus(s Status) string {
	switch s {
	case ExtremeRegression:
		return "Extreme"
	case SignificantRegression:
		return "Significant"
	case ExtremeTriagedRegression:
		return "ExtremeTriaged"
	case SignificantTriagedRegression:
		return "SignificantTriaged"
	case MissingSample:
		return "MissingSample"
	case FixedRegression:
		return "Fixed"
	case FailedFixedRegression:
		return "FailedFixed"
	}
	return "Unknown"
}

// JobVariants contains all variants supported in the system.
type JobVariants struct {
	Variants map[string][]string `json:"variants,omitempty"`
}

// KeyWithVariants connects the core unique db testID string to a set of variants.
// Used to serialize/deserialize as a map key when we pass test status around.
type KeyWithVariants struct {
	TestID string `json:"test_id"`

	// Proposed, need to serialize to use as map key
	Variants map[string]string `json:"variants"`
}

// KeyOrDie serializes this test key into a json string suitable for use in maps.
// JSON serialization uses sorted map keys, so the output is stable.
func (t KeyWithVariants) KeyOrDie() string {
	testIDBytes, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	return string(testIDBytes)
}

type ReleaseTimeRange struct {
	Release string
	End     *time.Time
	Start   *time.Time
}
