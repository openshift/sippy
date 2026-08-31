// Package labels applies an externally-sourced job run label request to Sippy's
// PostgreSQL state. It is the business logic behind the POST
// /api/job/run/labels endpoint.
//
// Every label is applied the same way: it is appended to the target run's
// prow_job_runs.labels array. Some labels then need extra bookkeeping (an
// InfraFailure label also removes the run's contribution from the pre-aggregated
// summary tables via pkg/db/infrafailure). The append and any such side effect
// run in a single transaction, and the per-label switch in applySideEffects is
// the one extension point for teaching this endpoint about new labels with
// special semantics.
package labels

import (
	"context"
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/infrafailure"
	"github.com/openshift/sippy/pkg/db/models"
)

// ApplyRequest is the request body for applying one job run label.
type ApplyRequest struct {
	RunID        string    `json:"run_id"`
	Label        string    `json:"label"`
	RequestedAt  time.Time `json:"requested_at"`
	ProwJobStart time.Time `json:"prowjob_start"`
	Release      string    `json:"release"`
	Comment      string    `json:"comment,omitempty"`
	User         string    `json:"user,omitempty"`
	SourceTool   string    `json:"source_tool,omitempty"`
	SymptomID    string    `json:"symptom_id,omitempty"`
}

// ApplyOutcome describes the internal result of applying a label. It is
// returned separately from Result so transport code can choose an HTTP status
// without exposing that implementation detail in the response body.
type ApplyOutcome int

const (
	// ApplyOutcomeError is the defensive zero value and means the request could
	// not be applied.
	ApplyOutcomeError ApplyOutcome = iota
	// ApplyOutcomeRecorded means the label was newly applied to the run. For an
	// InfraFailure label it also means the run's contribution was subtracted
	// from the summary tables.
	ApplyOutcomeRecorded
	// ApplyOutcomeAlreadyLabeled means the run already carried the label, so no change
	// was needed (idempotent no-op).
	ApplyOutcomeAlreadyLabeled
	// ApplyOutcomeRunNotFound means no prow_job_runs row exists for the run id.
	ApplyOutcomeRunNotFound
)

// Result is the JSON body returned by POST /api/job/run/labels. The message
// provides useful outcome detail while the HTTP status carries the
// machine-readable outcome.
type Result struct {
	RunID   string `json:"run_id"`
	Label   string `json:"label"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ValidateRequest checks a label request before any database work is done.
func ValidateRequest(request ApplyRequest) error {
	if request.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	if request.Label == "" {
		return fmt.Errorf("label is required")
	}
	// The label drives a database write keyed by the numeric prow_job_runs.id,
	// so run_id must be a valid int64 up front.
	if _, err := strconv.ParseInt(request.RunID, 10, 64); err != nil {
		return fmt.Errorf("invalid run_id %q: must be numeric", request.RunID)
	}
	// prowjob_start is required: downstream processing relies on the job run's
	// start time for partition-pruned lookups, so a request without it cannot be
	// processed efficiently downstream.
	if request.ProwJobStart.IsZero() {
		return fmt.Errorf("prowjob_start is required")
	}
	if request.Release == "" {
		return fmt.Errorf("release is required")
	}
	return nil
}

// appendOutcome describes what appending a label to prow_job_runs.labels did, so
// a run that is missing from PostgreSQL can be distinguished from one that
// already carried the label (both leave the guarded UPDATE with
// RowsAffected == 0).
type appendOutcome int

const (
	// outcomeUnknown is the zero value, reserved so an error path never
	// accidentally matches a meaningful outcome.
	outcomeUnknown appendOutcome = iota
	// outcomeApplied means the label was newly appended to the run.
	outcomeApplied
	// outcomeAlreadyPresent means the run already carried the label.
	outcomeAlreadyPresent
	// outcomeRunNotFound means no prow_job_runs row exists for the id.
	outcomeRunNotFound
)

// Applier applies label requests to PostgreSQL.
type Applier struct {
	dbc *db.DB

	// subtractInfraFailure enables testing InfraFailure side effects without a database.
	subtractInfraFailure func(tx *gorm.DB, prowJobRunID int64) error
}

// NewApplier constructs an Applier wired to the given database client.
func NewApplier(dbc *db.DB) *Applier {
	return &Applier{
		dbc:                  dbc,
		subtractInfraFailure: infrafailure.SubtractInfraFailureFromSummaries,
	}
}

// Apply records the label request in PostgreSQL and returns the outcome. The
// label append and any per-label side effect run in one transaction so they
// commit (or roll back) atomically.
func (a *Applier) Apply(ctx context.Context, request ApplyRequest) (Result, ApplyOutcome) {
	res := Result{RunID: request.RunID, Label: request.Label}

	if err := ValidateRequest(request); err != nil {
		res.Error = err.Error()
		return res, ApplyOutcomeError
	}

	runID, err := strconv.ParseInt(request.RunID, 10, 64)
	if err != nil {
		res.Error = fmt.Sprintf("invalid run_id %q: must be numeric", request.RunID)
		return res, ApplyOutcomeError
	}
	if a == nil || a.dbc == nil || a.dbc.DB == nil {
		res.Error = "applying labels requires a database connection"
		return res, ApplyOutcomeError
	}

	var outcome appendOutcome
	if err := a.dbc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var txErr error
		outcome, txErr = a.applyOne(tx, runID, request.Label)
		return txErr
	}); err != nil {
		log.WithError(err).WithField("run_id", res.RunID).WithField("label", request.Label).Error("failed to apply label")
		res.Error = err.Error()
		return res, ApplyOutcomeError
	}

	applyOutcome := applyOutcomeForAppendOutcome(outcome)
	res.Message = messageForApplyOutcome(applyOutcome)
	if applyOutcome == ApplyOutcomeError {
		res.Error = fmt.Sprintf("unexpected outcome applying label %q", request.Label)
	}
	return res, applyOutcome
}

// applyOne is the single label-application path shared by every label. It appends
// the label to the run's prow_job_runs.labels array and then runs any per-label
// side effect. Both steps run on tx so they commit atomically.
func (a *Applier) applyOne(tx *gorm.DB, runID int64, label string) (appendOutcome, error) {
	outcome, err := appendProwJobRunLabel(tx, runID, label)
	if err != nil {
		return outcomeUnknown, err
	}
	if err := a.applySideEffects(tx, outcome, runID, label); err != nil {
		return outcomeUnknown, err
	}
	return outcome, nil
}

// applySideEffects dispatches per-label bookkeeping that must commit in the same
// transaction as the label append. It is the single extension point for teaching
// this endpoint about labels with special semantics.
//
// Side effects run only when the label was newly applied (outcome ==
// outcomeApplied). An already-present label means the side effect already ran, so
// skipping it keeps the operation idempotent under request redelivery; a
// missing run has nothing to act on.
func (a *Applier) applySideEffects(tx *gorm.DB, outcome appendOutcome, runID int64, label string) error {
	if outcome != outcomeApplied {
		return nil
	}
	switch label {
	case infrafailure.LabelInfraFailure:
		// "InfraFailure label present == summary subtraction done": the label was
		// just newly appended, so remove the run's contribution from the summary
		// tables in the same transaction to uphold that invariant.
		if a == nil || a.subtractInfraFailure == nil {
			return fmt.Errorf("InfraFailure summary subtraction is unavailable")
		}
		return a.subtractInfraFailure(tx, runID)
	default:
		// Generic labels need no bookkeeping beyond the append. Add a case here to
		// give a future label special semantics; it will run in the same
		// transaction as the append.
		return nil
	}
}

// appendProwJobRunLabel idempotently appends label to a run's
// prow_job_runs.labels array on the supplied transaction. The guarded UPDATE is
// the array-column equivalent of ON CONFLICT DO NOTHING: array_append runs only
// when the label is not already present (the NULL-safe predicate also handles
// runs with no labels yet), so redelivery of the same request is a no-op.
func appendProwJobRunLabel(tx *gorm.DB, prowJobRunID int64, label string) (appendOutcome, error) {
	res := tx.Model(&models.ProwJobRun{}).
		Where("id = ? AND (labels IS NULL OR NOT (labels @> ARRAY[?]))", prowJobRunID, label).
		Update("labels", gorm.Expr("array_append(labels, ?)", label))
	if res.Error != nil {
		return outcomeUnknown, fmt.Errorf("appending label %q to prow_job_run %d: %w", label, prowJobRunID, res.Error)
	}
	if res.RowsAffected == 0 {
		// RowsAffected == 0 means either the label was already present or the run
		// does not exist. Disambiguate with an existence check so callers can
		// report run_not_found separately from already_labeled.
		var count int64
		if err := tx.Model(&models.ProwJobRun{}).
			Where("id = ?", prowJobRunID).Count(&count).Error; err != nil {
			return outcomeUnknown, fmt.Errorf("checking existence of prow_job_run %d: %w", prowJobRunID, err)
		}
		if count == 0 {
			return outcomeRunNotFound, nil
		}
		return outcomeAlreadyPresent, nil
	}
	log.WithFields(log.Fields{"prow_job_run_id": prowJobRunID, "label": label}).
		Debug("appended label to prow job run")
	return outcomeApplied, nil
}

// applyOutcomeForAppendOutcome maps the database append result to the outcome
// returned alongside the response body. outcomeUnknown maps to
// ApplyOutcomeError as a defensive fallback; in
// practice a successful append never yields it.
func applyOutcomeForAppendOutcome(outcome appendOutcome) ApplyOutcome {
	switch outcome {
	case outcomeApplied:
		return ApplyOutcomeRecorded
	case outcomeAlreadyPresent:
		return ApplyOutcomeAlreadyLabeled
	case outcomeRunNotFound:
		return ApplyOutcomeRunNotFound
	default:
		return ApplyOutcomeError
	}
}

func messageForApplyOutcome(outcome ApplyOutcome) string {
	switch outcome {
	case ApplyOutcomeRecorded:
		return "label recorded"
	case ApplyOutcomeAlreadyLabeled:
		return "label already present"
	case ApplyOutcomeRunNotFound:
		return "job run not found"
	default:
		return ""
	}
}
