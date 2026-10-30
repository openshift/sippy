package prowloader

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api/labels"
	"github.com/openshift/sippy/pkg/apis/prow"
)

func TestPrefetchLabelsLoadsAuthoritativeLabelsByBuildID(t *testing.T) {
	readCalls := 0
	loader := &ProwLoader{
		ctx:       context.Background(),
		gcsClient: &storage.Client{},
		readGCSLabels: func(_ context.Context, _ *storage.Client, bucket, jobPath string) (map[string]labels.DiscoveredLabel, error) {
			readCalls++
			if bucket != "results" || jobPath != "logs/job/123" {
				t.Fatalf("GCS location = (%q, %q)", bucket, jobPath)
			}
			return map[string]labels.DiscoveredLabel{
				"KnownFlake":   {ID: "KnownFlake"},
				"InfraFailure": {ID: "InfraFailure"},
			}, nil
		},
	}
	jobs := []prow.ProwJob{{
		Spec:   prow.ProwJobSpec{Job: "job", DecorationConfig: prow.DecorationConfig{GCSConfiguration: prow.GCSConfiguration{Bucket: "results"}}},
		Status: prow.ProwJobStatus{BuildID: "123", URL: "https://prow.ci/view/gs/results/logs/job/123"},
	}}

	got, err := loader.prefetchLabels(jobs)
	if err != nil {
		t.Fatal(err)
	}
	if readCalls != 1 {
		t.Fatalf("GCS label reads = %d, want 1", readCalls)
	}
	if len(got) != 1 {
		t.Fatalf("labels by build ID = %#v, want one build ID", got)
	}
	if len(got["123"]) != 2 || got["123"][0] != "InfraFailure" || got["123"][1] != "KnownFlake" {
		t.Fatalf("labels = %#v", got)
	}
}

func TestPrefetchLabelsPropagatesGCSReadError(t *testing.T) {
	expectedErr := errors.New("GCS unavailable")
	loader := &ProwLoader{
		ctx:       context.Background(),
		gcsClient: &storage.Client{},
		readGCSLabels: func(context.Context, *storage.Client, string, string) (map[string]labels.DiscoveredLabel, error) {
			return nil, expectedErr
		},
	}
	jobs := []prow.ProwJob{{
		Status: prow.ProwJobStatus{BuildID: "123", URL: "https://prow.ci/view/gs/results/logs/job/123"},
	}}

	_, err := loader.prefetchLabels(jobs)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("error = %v, want wrapped GCS read error", err)
	}
}

func TestPrefetchLabelsFailsWithoutGCSClient(t *testing.T) {
	loader := &ProwLoader{ctx: context.Background()}
	if _, err := loader.prefetchLabels(nil); err == nil {
		t.Fatal("expected missing GCS client error")
	}
}
