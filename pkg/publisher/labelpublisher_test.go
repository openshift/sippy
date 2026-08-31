package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
)

// fieldsEqual compares two LabelEvents, using time.Time.Equal for the time
// fields so a JSON round-trip through RFC 3339 does not cause a spurious
// mismatch on the monotonic-clock or location components.
func fieldsEqual(a, b LabelEvent) bool {
	return a.RunID == b.RunID && a.Label == b.Label && a.RequestedAt.Equal(b.RequestedAt) &&
		a.ProwJobStart.Equal(b.ProwJobStart) && a.Release == b.Release &&
		a.Comment == b.Comment && a.User == b.User &&
		a.SourceTool == b.SourceTool && a.SymptomID == b.SymptomID
}

func TestLabelEventReleaseJSONFieldIsRequired(t *testing.T) {
	data, err := json.Marshal(LabelEvent{})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := fields["release"]; !ok {
		t.Fatal("LabelEvent JSON omitted required release field")
	}
}

func TestPublishLabel(t *testing.T) {
	event := LabelEvent{
		RunID:        "111",
		Label:        "InfraFailure",
		RequestedAt:  time.Unix(100, 0).UTC(),
		ProwJobStart: time.Unix(50, 0).UTC(),
		Release:      "4.18",
		Comment:      "flaky infra, see triage thread",
		User:         "jdoe",
		SourceTool:   "sippy-ui",
		SymptomID:    "sym-42",
	}

	var published []*pubsub.Message
	p := &LabelPublisher{
		publish: func(_ context.Context, msg *pubsub.Message) (string, error) {
			published = append(published, msg)
			return "server-id", nil
		},
	}

	if err := ValidateLabelEvent(event); err != nil {
		t.Fatalf("ValidateLabelEvent() error = %v", err)
	}
	if err := p.PublishLabel(context.Background(), event); err != nil {
		t.Fatalf("PublishLabel() error = %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("published %d messages, want 1", len(published))
	}
	msg := published[0]
	var got LabelEvent
	if err := json.Unmarshal(msg.Data, &got); err != nil {
		t.Fatalf("message data is not valid LabelEvent JSON: %v", err)
	}
	if !fieldsEqual(got, event) {
		t.Errorf("message decoded = %+v, want %+v", got, event)
	}
	if msg.Attributes["run_id"] != event.RunID {
		t.Errorf("run_id attribute = %q, want %q", msg.Attributes["run_id"], event.RunID)
	}
	if msg.Attributes["label"] != event.Label {
		t.Errorf("label attribute = %q, want %q", msg.Attributes["label"], event.Label)
	}
	if msg.Attributes["source_tool"] != event.SourceTool {
		t.Errorf("source_tool attribute = %q, want %q", msg.Attributes["source_tool"], event.SourceTool)
	}
	if got, want := msg.Attributes["prowjob_start"], event.ProwJobStart.Format(time.RFC3339); got != want {
		t.Errorf("prowjob_start attribute = %q, want %q", got, want)
	}
	if got, want := msg.Attributes["release"], event.Release; got != want {
		t.Errorf("release attribute = %q, want %q", got, want)
	}
}

func TestPublishLabelError(t *testing.T) {
	wantErr := errors.New("boom")
	calls := 0
	p := &LabelPublisher{
		publish: func(_ context.Context, _ *pubsub.Message) (string, error) {
			calls++
			return "", wantErr
		},
	}
	err := p.PublishLabel(context.Background(), LabelEvent{
		RunID:        "1",
		Label:        "InfraFailure",
		ProwJobStart: time.Unix(50, 0).UTC(),
		Release:      "4.18",
	})
	if err == nil {
		t.Fatal("PublishLabel() expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want wrapping %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("expected exactly one publish call, got %d", calls)
	}
}

func TestPublishLabelRejectsInvalidRequiredEvent(t *testing.T) {
	validEvent := LabelEvent{
		RunID:        "1",
		Label:        "InfraFailure",
		ProwJobStart: time.Unix(50, 0).UTC(),
		Release:      "4.18",
	}
	tests := []struct {
		name    string
		mutate  func(*LabelEvent)
		errText string
	}{
		{name: "missing run_id", mutate: func(event *LabelEvent) { event.RunID = "" }, errText: "run_id is required"},
		{name: "non-numeric run_id", mutate: func(event *LabelEvent) { event.RunID = "not-a-number" }, errText: `invalid run_id "not-a-number": must be numeric`},
		{name: "missing label", mutate: func(event *LabelEvent) { event.Label = "" }, errText: "label is required"},
		{name: "missing prowjob_start", mutate: func(event *LabelEvent) { event.ProwJobStart = time.Time{} }, errText: "prowjob_start is required"},
		{name: "missing release", mutate: func(event *LabelEvent) { event.Release = "" }, errText: "release is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent
			tt.mutate(&event)
			if err := ValidateLabelEvent(event); err == nil || err.Error() != tt.errText {
				t.Fatalf("ValidateLabelEvent() error = %v, want %q", err, tt.errText)
			}
			calls := 0
			p := &LabelPublisher{
				publish: func(_ context.Context, _ *pubsub.Message) (string, error) {
					calls++
					return "server-id", nil
				},
			}

			err := p.PublishLabel(context.Background(), event)
			if err == nil || err.Error() != tt.errText {
				t.Fatalf("PublishLabel() error = %v, want %q", err, tt.errText)
			}
			if calls != 0 {
				t.Errorf("publish called %d times, want 0", calls)
			}
		})
	}
}

func TestNewLabelPublisherWiresPublish(t *testing.T) {
	p := NewLabelPublisher(&pubsub.Publisher{})
	if p.publish == nil {
		t.Fatal("NewLabelPublisher did not wire a publish function")
	}
}

func TestPublishLabelRejectsInvalidPublisherConfiguration(t *testing.T) {
	validEvent := LabelEvent{
		RunID:        "1",
		Label:        "InfraFailure",
		ProwJobStart: time.Unix(50, 0).UTC(),
		Release:      "4.18",
	}
	tests := []struct {
		name      string
		publisher *LabelPublisher
	}{
		{name: "nil constructor argument", publisher: NewLabelPublisher(nil)},
		{name: "zero value", publisher: &LabelPublisher{}},
		{name: "nil publish seam", publisher: &LabelPublisher{publish: nil}},
		{name: "nil receiver", publisher: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.publisher.PublishLabel(context.Background(), validEvent)
			if !errors.Is(err, ErrLabelPublisherNotConfigured) {
				t.Fatalf("PublishLabel() error = %v, want %v", err, ErrLabelPublisherNotConfigured)
			}
		})
	}
}

func TestPublishLabelValidatesBeforePublisherConfiguration(t *testing.T) {
	var p *LabelPublisher
	err := p.PublishLabel(context.Background(), LabelEvent{})
	if err == nil || err.Error() != "run_id is required" {
		t.Fatalf("PublishLabel() error = %v, want %q", err, "run_id is required")
	}
}
