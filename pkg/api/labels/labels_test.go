package labels

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/infrafailure"
)

func TestApplyRequestJSONFields(t *testing.T) {
	type expectedField struct {
		name string
		tag  string
	}
	want := []expectedField{
		{name: "RunID", tag: "run_id"},
		{name: "Label", tag: "label"},
		{name: "RequestedAt", tag: "requested_at"},
		{name: "ProwJobStart", tag: "prowjob_start"},
		{name: "Release", tag: "release"},
		{name: "Comment", tag: "comment,omitempty"},
		{name: "User", tag: "user,omitempty"},
		{name: "SourceTool", tag: "source_tool,omitempty"},
		{name: "SymptomID", tag: "symptom_id,omitempty"},
	}

	requestType := reflect.TypeOf(ApplyRequest{})
	if requestType.NumField() != len(want) {
		t.Fatalf("ApplyRequest has %d fields, want %d", requestType.NumField(), len(want))
	}
	for _, field := range want {
		t.Run(field.name, func(t *testing.T) {
			got, ok := requestType.FieldByName(field.name)
			if !ok {
				t.Fatalf("ApplyRequest is missing field %s", field.name)
			}
			if tag := got.Tag.Get("json"); tag != field.tag {
				t.Errorf("ApplyRequest.%s JSON tag = %q, want %q", field.name, tag, field.tag)
			}
		})
	}
}

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		request ApplyRequest
		wantErr bool
		errMsg  string
	}{
		{name: "missing run_id", request: ApplyRequest{Label: "InfraFailure"}, wantErr: true, errMsg: "run_id is required"},
		{name: "missing label", request: ApplyRequest{RunID: "1"}, wantErr: true, errMsg: "label is required"},
		{name: "non-numeric run_id for InfraFailure", request: ApplyRequest{RunID: "abc", Label: "InfraFailure"}, wantErr: true, errMsg: "must be numeric"},
		// Every label is recorded against the numeric prow_job_runs.id, so a
		// non-numeric run_id is rejected regardless of the label.
		{name: "non-numeric run_id for other label", request: ApplyRequest{RunID: "abc", Label: "DNSTimeout"}, wantErr: true, errMsg: "must be numeric"},
		// prowjob_start is required because downstream processing needs the job
		// run's start time for partition-pruned lookups.
		{name: "missing prowjob_start", request: ApplyRequest{RunID: "123", Label: "InfraFailure"}, wantErr: true, errMsg: "prowjob_start is required"},
		{name: "missing release", request: ApplyRequest{RunID: "123", Label: "InfraFailure", ProwJobStart: time.Unix(1000, 0).UTC()}, wantErr: true, errMsg: "release is required"},
		{name: "valid InfraFailure", request: ApplyRequest{RunID: "123", Label: "InfraFailure", ProwJobStart: time.Unix(1000, 0).UTC(), Release: "4.18"}, wantErr: false},
		{name: "valid other label", request: ApplyRequest{RunID: "123", Label: "DNSTimeout", ProwJobStart: time.Unix(1000, 0).UTC(), Release: "4.18"}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequest(tt.request)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

// TestApplySideEffects covers the single per-label side-effect switch: an
// InfraFailure label that was newly applied triggers the summary-subtraction
// seam, while every other label does not. The outcome gate is what keeps this
// idempotent, so the cases also assert that an already-present or not-found
// InfraFailure label never subtracts (a redelivered request must not double
// subtract). Passing a nil *gorm.DB is safe because the fake seam ignores it and
// the default case never touches it, so the dispatch is exercised without a
// database.
func TestApplySideEffects(t *testing.T) {
	tests := []struct {
		name         string
		outcome      appendOutcome
		label        string
		subtractErr  error
		wantSubtract bool
		wantErr      bool
	}{
		{name: "infra newly applied subtracts", outcome: outcomeApplied, label: infrafailure.LabelInfraFailure, wantSubtract: true},
		{name: "infra already present does not subtract", outcome: outcomeAlreadyPresent, label: infrafailure.LabelInfraFailure, wantSubtract: false},
		{name: "infra run not found does not subtract", outcome: outcomeRunNotFound, label: infrafailure.LabelInfraFailure, wantSubtract: false},
		{name: "generic newly applied has no side effect", outcome: outcomeApplied, label: "DNSTimeout", wantSubtract: false},
		{name: "generic already present has no side effect", outcome: outcomeAlreadyPresent, label: "NodeProblem", wantSubtract: false},
		{name: "subtraction error propagates", outcome: outcomeApplied, label: infrafailure.LabelInfraFailure, subtractErr: errors.New("db down"), wantSubtract: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			a := &Applier{
				subtractInfraFailure: func(_ *gorm.DB, _ int64) error {
					called = true
					return tt.subtractErr
				},
			}
			err := a.applySideEffects(nil, tt.outcome, 123, tt.label)
			if (err != nil) != tt.wantErr {
				t.Fatalf("applySideEffects() error = %v, wantErr %v", err, tt.wantErr)
			}
			if called != tt.wantSubtract {
				t.Errorf("subtractInfraFailure called = %v, want %v", called, tt.wantSubtract)
			}
		})
	}
}

// TestApplyOutcomeForAppendOutcome verifies the append-result to apply-outcome mapping,
// including the defensive fallback for the zero value.
func TestApplyOutcomeForAppendOutcome(t *testing.T) {
	tests := []struct {
		name    string
		outcome appendOutcome
		want    ApplyOutcome
	}{
		{name: "applied", outcome: outcomeApplied, want: ApplyOutcomeRecorded},
		{name: "already present", outcome: outcomeAlreadyPresent, want: ApplyOutcomeAlreadyLabeled},
		{name: "run not found", outcome: outcomeRunNotFound, want: ApplyOutcomeRunNotFound},
		{name: "unknown falls back to error", outcome: outcomeUnknown, want: ApplyOutcomeError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyOutcomeForAppendOutcome(tt.outcome); got != tt.want {
				t.Errorf("applyOutcomeForAppendOutcome(%d) = %d, want %d", int(tt.outcome), got, tt.want)
			}
		})
	}
}

// TestApplyRejectsNonNumericRunID verifies a non-numeric run_id fails before any
// database work: the result is an error and the side-effect seam is never
// reached (so a nil dbc is safe in this test).
func TestApplyRejectsNonNumericRunID(t *testing.T) {
	called := false
	a := &Applier{
		subtractInfraFailure: func(_ *gorm.DB, _ int64) error {
			called = true
			return nil
		},
	}
	got, outcome := a.Apply(context.Background(), ApplyRequest{RunID: "xyz", Label: infrafailure.LabelInfraFailure})
	if outcome != ApplyOutcomeError {
		t.Errorf("outcome = %d, want %d", outcome, ApplyOutcomeError)
	}
	if !strings.Contains(got.Error, "must be numeric") {
		t.Errorf("error %q does not contain %q", got.Error, "must be numeric")
	}
	if called {
		t.Error("subtractInfraFailure should not be called for an invalid run_id")
	}
	if got.RunID != "xyz" || got.Label != infrafailure.LabelInfraFailure {
		t.Errorf("result run_id/label = %q/%q, want %q/%q", got.RunID, got.Label, "xyz", infrafailure.LabelInfraFailure)
	}
}
