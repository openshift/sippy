package infrafailure

import "testing"

// The DB-backed behavior of RecordInfraFailure and RecordInfraFailureWithOutcome
// (label gating, summary subtraction, idempotency, and the per-run outcome
// classification) requires a real PostgreSQL instance and is exercised by the
// integration tests in test/integration/infrafailure_test.go. The unit tests
// below cover the parts that do not need a database: the RecordOutcome enum and
// its String rendering.

// TestRecordOutcomeValues guards the iota ordering: OutcomeUnknown must remain
// the zero value (so an uninitialized or error-path RecordOutcome never
// masquerades as a meaningful outcome such as OutcomeSubtracted), and the
// remaining outcomes must stay distinct and in order.
func TestRecordOutcomeValues(t *testing.T) {
	var zero RecordOutcome
	if zero != OutcomeUnknown {
		t.Fatalf("zero value = %d, want OutcomeUnknown", int(zero))
	}

	tests := []struct {
		name string
		got  RecordOutcome
		want int
	}{
		{name: "unknown", got: OutcomeUnknown, want: 0},
		{name: "subtracted", got: OutcomeSubtracted, want: 1},
		{name: "already labeled", got: OutcomeAlreadyLabeled, want: 2},
		{name: "run not found", got: OutcomeRunNotFound, want: 3},
	}
	for _, tc := range tests {
		if int(tc.got) != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, int(tc.got), tc.want)
		}
	}
}

// TestRecordOutcomeString verifies the human-readable rendering used in log
// fields, including the fallback for unrecognized values.
func TestRecordOutcomeString(t *testing.T) {
	tests := []struct {
		name    string
		outcome RecordOutcome
		want    string
	}{
		{name: "unknown", outcome: OutcomeUnknown, want: "unknown"},
		{name: "subtracted", outcome: OutcomeSubtracted, want: "subtracted"},
		{name: "already labeled", outcome: OutcomeAlreadyLabeled, want: "already-labeled"},
		{name: "run not found", outcome: OutcomeRunNotFound, want: "run-not-found"},
		{name: "unrecognized falls back", outcome: RecordOutcome(99), want: "RecordOutcome(99)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.outcome.String(); got != tc.want {
				t.Errorf("RecordOutcome(%d).String() = %q, want %q", int(tc.outcome), got, tc.want)
			}
		})
	}
}
