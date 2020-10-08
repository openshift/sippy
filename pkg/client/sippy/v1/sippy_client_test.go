package v1

import (
	"context"
	"testing"
)

func TestReleaseReportClient(t *testing.T) {
	c := New()
	report, err := c.Release("4.6").Report(context.TODO())
	if err != nil {
		t.Fatalf("unable to fetch 4.6 report: %v", err)
	}
	if c := len(report.CanaryTestFailures); c == 0 {
		t.Errorf("expected some canary test failures, got %d", c)
	}
	if c := len(report.JobPassRateByPlatform); c == 0 {
		t.Errorf("expected job pass rates by platform, got %d", c)
	}
}
