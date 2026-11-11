package metrics

import (
	"context"
	"strings"
	"testing"
	"time"

	cloudbq "cloud.google.com/go/bigquery"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util"
)

func TestRefreshDisruptionMetricsCancelsActiveReportLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	client := &bqclient.Client{
		BQ:    &cloudbq.Client{},
		Cache: &util.PseudoCache{Cache: map[string][]byte{}},
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- refreshDisruptionMetrics(ctx, client, nil, func(loadCtx context.Context, _ *bqclient.Client) (apitype.DisruptionReport, []error) {
			close(started)
			<-loadCtx.Done()
			return apitype.DisruptionReport{}, []error{loadCtx.Err()}
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("disruption report load did not start")
	}

	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("refreshDisruptionMetrics() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("disruption report load did not stop after context cancellation")
	}
}
