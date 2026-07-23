package crtest

import (
	"sort"
	"strings"
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

// VariantKeyValueToString formats a variant key and value as "key:value".
func VariantKeyValueToString(key, value string) string {
	return key + ":" + value
}

// VariantStringToKeyValue splits a "key:value" string into its key and value.
// Returns empty strings if the format is invalid.
func VariantStringToKeyValue(variant string) (string, string) {
	k, v, ok := strings.Cut(variant, ":")
	if !ok {
		return "", ""
	}
	return k, v
}

// EncodeVariants returns a deterministic null-byte-separated encoding of variant pairs.
func EncodeVariants(variants map[string]string) string {
	pairs := make([]string, 0, len(variants))
	for k, v := range variants {
		pairs = append(pairs, VariantKeyValueToString(k, v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "\x00")
}

// Encode returns a deterministic string encoding suitable for use as a map key.
// Format: testID\x00key1:val1\x00key2:val2 (sorted variant pairs, null-separated).
func (t KeyWithVariants) Encode() string {
	encoded := EncodeVariants(t.Variants)
	if encoded == "" {
		return t.TestID
	}
	return t.TestID + "\x00" + encoded
}

// Encode returns a deterministic string encoding for column identification.
// Format: key1:val1\x00key2:val2 (sorted variant pairs, null-separated).
func (c ColumnIdentification) Encode() ColumnID {
	return ColumnID(EncodeVariants(c.Variants))
}

// DecodeColumnID reverses ColumnIdentification.Encode().
func DecodeColumnID(key ColumnID) ColumnIdentification {
	variants := make(map[string]string)
	if key == "" {
		return ColumnIdentification{Variants: variants}
	}
	for p := range strings.SplitSeq(string(key), "\x00") {
		if k, v := VariantStringToKeyValue(p); k != "" {
			variants[k] = v
		}
	}
	return ColumnIdentification{Variants: variants}
}

type ReleaseTimeRange struct {
	Release string
	End     *time.Time
	Start   *time.Time
}
