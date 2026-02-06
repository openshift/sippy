package jobrunevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
)

// eventsJSONRegex matches paths like artifacts/*e2e*/gather-extra/artifacts/events.json
var eventsJSONRegex = regexp.MustCompile(`gather-extra/artifacts/events\.json$`)

// KubeEvent represents a flattened Kubernetes Event for the API response
type KubeEvent struct {
	FirstTimestamp string `json:"firstTimestamp"`
	LastTimestamp  string `json:"lastTimestamp"`
	Namespace      string `json:"namespace"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Count          int    `json:"count"`
	Source         string `json:"source"`
}

// rawKubeEvent is the Kubernetes Event structure from events.json
type rawKubeEvent struct {
	FirstTimestamp string                 `json:"firstTimestamp"`
	LastTimestamp  string                 `json:"lastTimestamp"`
	EventTime      string                 `json:"eventTime"`
	InvolvedObject map[string]interface{} `json:"involvedObject"`
	Type           string                 `json:"type"`
	Reason         string                 `json:"reason"`
	Message        string                 `json:"message"`
	Count          int                    `json:"count"`
	Source         map[string]string      `json:"source"`
	Metadata       map[string]interface{} `json:"metadata"`
	ReportingComp  string                 `json:"reportingComponent"`
}

// EventListResponse is the API response for job run events
type EventListResponse struct {
	Items     []KubeEvent `json:"items"`
	JobRunURL string      `json:"jobRunURL"`
}

// JobRunEvents fetches events.json for a given job run from the GCS path
// artifacts/*e2e*/gather-extra/artifacts/events.json
func JobRunEvents(gcsClient *storage.Client, dbc *db.DB, jobRunID int64, gcsBucket, gcsPath string, logger *log.Entry) (*EventListResponse, error) {
	jobRunURL := fmt.Sprintf("https://prow.ci.openshift.org/view/gs/%s/%s", gcsBucket, gcsPath)

	jobRun, err := api.FetchJobRun(dbc, jobRunID, false, nil, logger)
	if err != nil {
		logger.WithError(err).Debugf("failed to fetch job run %d", jobRunID)
		if gcsPath == "" {
			return nil, errors.New("no GCS path given and no job run found in DB")
		}
	} else {
		jobRunURL = jobRun.URL
		gcsBucket = jobRun.GCSBucket
		_, path, found := strings.Cut(jobRunURL, "/"+gcsBucket+"/")
		if !found {
			return nil, fmt.Errorf("job run URL %q does not contain bucket %q", jobRun.URL, gcsBucket)
		}
		gcsPath = path
	}

	gcsJobRun := gcs.NewGCSJobRun(gcsClient.Bucket(gcsBucket), gcsPath)
	matches, err := gcsJobRun.FindAllMatches([]*regexp.Regexp{eventsJSONRegex})
	if err != nil {
		return &EventListResponse{JobRunURL: jobRunURL}, err
	}

	if len(matches) == 0 || len(matches[0]) == 0 {
		logger.Info("no events.json file found")
		return &EventListResponse{Items: []KubeEvent{}, JobRunURL: jobRunURL}, nil
	}

	eventsPath := matches[0][0]
	logger.WithField("events_path", eventsPath).Info("found events.json")

	content, err := gcsJobRun.GetContent(context.TODO(), eventsPath)
	if err != nil {
		logger.WithError(err).Errorf("error getting content for file: %s", eventsPath)
		return nil, err
	}

	var rawEvents struct {
		Items []rawKubeEvent `json:"items"`
	}
	if err := json.Unmarshal(content, &rawEvents); err != nil {
		logger.WithError(err).Error("error unmarshaling events.json")
		return nil, err
	}

	events := make([]KubeEvent, 0, len(rawEvents.Items))
	for _, raw := range rawEvents.Items {
		evt := flattenEvent(raw)
		events = append(events, evt)
	}

	return &EventListResponse{Items: events, JobRunURL: jobRunURL}, nil
}

func flattenEvent(raw rawKubeEvent) KubeEvent {
	evt := KubeEvent{
		FirstTimestamp: raw.FirstTimestamp,
		LastTimestamp:  raw.LastTimestamp,
		Type:           raw.Type,
		Reason:         raw.Reason,
		Message:        raw.Message,
		Count:          raw.Count,
	}
	if raw.Count == 0 {
		evt.Count = 1
	}
	if raw.InvolvedObject != nil {
		if k, ok := raw.InvolvedObject["kind"].(string); ok {
			evt.Kind = k
		}
		if n, ok := raw.InvolvedObject["name"].(string); ok {
			evt.Name = n
		}
	}
	if raw.Metadata != nil {
		if ns, ok := raw.Metadata["namespace"].(string); ok {
			evt.Namespace = ns
		}
	}
	if raw.Source != nil && raw.Source["component"] != "" {
		evt.Source = raw.Source["component"]
	} else if raw.ReportingComp != "" {
		evt.Source = raw.ReportingComp
	}
	return evt
}
