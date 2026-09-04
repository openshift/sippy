// Package publisher publishes job run label events to a Google Cloud Pub/Sub
// topic so downstream subscribers (a BigQuery writer and a PostgreSQL sync
// worker) can react to labels applied by Sippy's label writers.
//
// The publisher is injected via NewLabelPublisher rather than constructed at
// package init so the dependency is explicit and the caller owns the Pub/Sub
// client lifecycle. The single Pub/Sub call the publisher makes is isolated
// behind a function-type field (the publish seam), following the project
// convention for making a narrow client call substitutable in tests without
// mocking the whole client (see pkg/dataloader/infrafailurebackfill.Backfiller).
package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"cloud.google.com/go/pubsub/v2"
)

// LabelEvent is the Pub/Sub event contract for a single job run label change.
//
// RunID is the prow job run build id carried as a string; it maps to the int64
// prow_job_runs.id primary key in PostgreSQL. It is kept as a string so the
// wire format stays stable regardless of JSON number handling.
//
// RunID, Label, ProwJobStart, and Release are required. ProwJobStart (the job
// run's start time) and Release (the OCP release) are partition keys subscribers
// use for efficient, partition-pruned lookups: the BigQuery job_labels table partitions
// on prowjob_start, and the PostgreSQL tables are (or soon will be) partitioned
// by prowjob_start and by release. The PostgreSQL label-application API applies
// the label using only RunID and Label.
//
// RequestedAt is when the label was requested/applied, which is distinct from
// ProwJobStart, the job run's own start time. Comment, User, SourceTool, and
// SymptomID are optional traceability metadata destined for the BigQuery
// job_labels table that not every label writer populates.
type LabelEvent struct {
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

// ValidateLabelEvent validates the fields required by all LabelEvent
// publishers and subscribers.
func ValidateLabelEvent(event LabelEvent) error {
	if event.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	if event.Label == "" {
		return fmt.Errorf("label is required")
	}
	if _, err := strconv.ParseInt(event.RunID, 10, 64); err != nil {
		return fmt.Errorf("invalid run_id %q: must be numeric", event.RunID)
	}
	if event.ProwJobStart.IsZero() {
		return fmt.Errorf("prowjob_start is required")
	}
	if event.Release == "" {
		return fmt.Errorf("release is required")
	}
	return nil
}

// publishFunc is the single Pub/Sub operation the publisher performs: publish
// one message and block for the server-assigned id (or an error). Wiring it as
// a field lets tests supply a closure instead of a real *pubsub.Publisher, which
// cannot be constructed or mocked through the public Pub/Sub API.
type publishFunc func(ctx context.Context, msg *pubsub.Message) (string, error)

// LabelPublisher publishes LabelEvents to a Pub/Sub topic.
type LabelPublisher struct {
	publish publishFunc
}

// ErrLabelPublisherNotConfigured indicates that no Pub/Sub publisher is configured.
var ErrLabelPublisherNotConfigured = errors.New("label publisher is not configured")

// NewLabelPublisher returns a LabelPublisher that publishes to the given publisher.
// The publisher is injected by the caller (dependency injection) rather than
// created here, so ownership of the Pub/Sub client and publisher lifecycle stays
// with the caller.
func NewLabelPublisher(publisher *pubsub.Publisher) *LabelPublisher {
	if publisher == nil {
		return &LabelPublisher{}
	}
	return &LabelPublisher{
		publish: func(ctx context.Context, msg *pubsub.Message) (string, error) {
			return publisher.Publish(ctx, msg).Get(ctx)
		},
	}
}

// PublishLabel serializes the LabelEvent to JSON and publishes it as one Pub/Sub
// message, blocking until the publish is acknowledged by the server. Publishing
// one message per event keeps events independently deliverable to each
// subscription. Required fields are validated before the event is marshaled or
// published. It returns the validation or publish error, if any.
func (p *LabelPublisher) PublishLabel(ctx context.Context, event LabelEvent) error {
	if err := ValidateLabelEvent(event); err != nil {
		return err
	}
	if p == nil || p.publish == nil {
		return ErrLabelPublisherNotConfigured
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling label event for run %s: %w", event.RunID, err)
	}
	// The routing-relevant fields are duplicated into message attributes so
	// subscriptions can filter without decoding the message body.
	msg := &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"run_id":        event.RunID,
			"label":         event.Label,
			"source_tool":   event.SourceTool,
			"prowjob_start": event.ProwJobStart.Format(time.RFC3339),
			"release":       event.Release,
		},
	}
	if _, err := p.publish(ctx, msg); err != nil {
		return fmt.Errorf("publishing label event for run %s: %w", event.RunID, err)
	}
	return nil
}
