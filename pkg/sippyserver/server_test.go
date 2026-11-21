package sippyserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestValidateCRDataSource(t *testing.T) {
	for _, dataSource := range []string{"", reqopts.DataSourceBigQuery, reqopts.DataSourcePostgres} {
		t.Run(dataSource, func(t *testing.T) {
			if err := validateCRDataSource(dataSource); err != nil {
				t.Fatalf("validateCRDataSource(%q) returned error: %v", dataSource, err)
			}
		})
	}

	err := validateCRDataSource("bigquey")
	if err == nil {
		t.Fatal("validateCRDataSource(bigquey) returned nil")
	}
	if !strings.Contains(err.Error(), "invalid dataSource") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "must be empty, \"bigquery\", or \"postgres\"") {
		t.Fatalf("error does not describe all valid values: %v", err)
	}
}

func TestValidateProwJobRun(t *testing.T) {

	tests := []struct {
		name                 string
		prowJobRun           *models.ProwJobRun
		expectedValidation   bool
		expectedDetailReason string
	}{
		{
			// no prowJobRun specified
			// simulates what we are seeing from the origin riskanalysis command
			// when missing junit artifacts
			name:                 "Test Nil ProwJobRun",
			expectedValidation:   false,
			expectedDetailReason: "empty ProwJobRun",
		},
		{
			prowJobRun:           &models.ProwJobRun{},
			name:                 "Test Empty ProwJobRun",
			expectedValidation:   false,
			expectedDetailReason: "missing ProwJob Name",
		},
		{
			prowJobRun:           &models.ProwJobRun{ProwJob: models.ProwJob{}},
			name:                 "Test Empty ProwJob",
			expectedValidation:   false,
			expectedDetailReason: "missing ProwJob Name",
		},
		{
			prowJobRun:         &models.ProwJobRun{ProwJob: models.ProwJob{Name: "test"}},
			name:               "Test Valid ProwJob",
			expectedValidation: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// no files found so we marshall the null prowJobRun
			inputBytes, err := json.Marshal(tc.prowJobRun)

			// we don't encounter an error
			if err != nil {
				t.Fatalf("Error marshalling prowjob for %s", tc.name)
			}

			// string 'null' not nil bytes if we have a nil pointer
			if inputBytes == nil {
				t.Fatalf("Nil Bytes for %s", tc.name)
			}

			jobRun := &models.ProwJobRun{}

			// we decode the string 'null' but we don't get an error...
			err = json.NewDecoder(strings.NewReader(string(inputBytes))).Decode(&jobRun)

			if err != nil {
				t.Fatalf("Error decoding prowjob for %s", tc.name)
			}

			isValid, detailReason := isValidProwJobRun(jobRun)

			if isValid != tc.expectedValidation {
				t.Fatalf("Validation %t did not match expected Expected %t for %s", isValid, tc.expectedValidation, tc.name)
			}

			if detailReason != tc.expectedDetailReason {
				t.Fatalf("DetailReason %s did not match Expected %s for %s", detailReason, tc.expectedDetailReason, tc.name)
			}

		})
	}

}

func TestEncodeDefaultHighRisk(t *testing.T) {
	result := apitype.ProwJobRunRiskAnalysis{
		OverallRisk: apitype.JobFailureRisk{
			Level:   apitype.FailureRiskLevelHigh,
			Reasons: []string{"Invalid ProwJob provided for analysis"},
		},
	}

	encodedRiskResult, err := json.Marshal(result)

	if err != nil {
		t.Fatal("Error while encoding risk analysis")
	}

	riskResultJSON := string(encodedRiskResult)

	if riskResultJSON == "" {
		t.Fatal("Invalid risk analysis json")
	}

	analysis := &apitype.ProwJobRunRiskAnalysis{}

	err = json.NewDecoder(strings.NewReader(string(encodedRiskResult))).Decode(&analysis)

	if err != nil {
		t.Fatal("Error while decoding risk analysis")
	}

	if analysis == nil {
		t.Fatal("Invalid risk analysis after decoding")
	}

	if analysis.OverallRisk.Level.Level != apitype.FailureRiskLevelHigh.Level {
		t.Fatal("Invalid overall risk analysis after decoding")
	}
}

func TestRefreshDataReportsCancellation(t *testing.T) {
	tests := []struct {
		name      string
		cancelCtx bool
		expectErr error
	}{
		{name: "active context reports success"},
		{name: "canceled context returns the context error", cancelCtx: true, expectErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancelCtx {
				cancel()
			}
			// a nil db skips the refresh queries, isolating the completion check
			err := RefreshData(ctx, nil, nil, RefreshOptions{})
			if tt.expectErr == nil {
				if err != nil {
					t.Fatalf("RefreshData returned error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.expectErr) {
				t.Fatalf("RefreshData error = %v, want %v", err, tt.expectErr)
			}
		})
	}
}

func TestContextWithDisconnectGrace(t *testing.T) {
	type ctxKey struct{}
	const grace = 5 * time.Minute
	tests := []struct {
		name         string
		cancelParent bool
		cancelChild  bool
		elapsed      time.Duration
		expectDone   bool
	}{
		{name: "live parent keeps the context open", elapsed: 2 * grace},
		{name: "parent cancellation is deferred during the grace period", cancelParent: true, elapsed: grace - time.Nanosecond},
		{name: "parent cancellation propagates once the grace period expires", cancelParent: true, elapsed: grace, expectDone: true},
		{name: "cancel function stops the context immediately", cancelChild: true, expectDone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// synctest's fake clock makes the grace boundary exact, independent of scheduling delays
			synctest.Test(t, func(t *testing.T) {
				parent, cancelParent := context.WithCancel(context.WithValue(t.Context(), ctxKey{}, "value"))
				defer cancelParent()
				ctx, cancel := contextWithDisconnectGrace(parent, grace)
				defer cancel()

				if got := ctx.Value(ctxKey{}); got != "value" {
					t.Fatalf("context value = %v, want parent value", got)
				}
				if tt.cancelParent {
					cancelParent()
				}
				if tt.cancelChild {
					cancel()
				}
				// let the cancellation callback start its grace timer before advancing the clock
				synctest.Wait()

				time.Sleep(tt.elapsed)
				synctest.Wait()

				if done := ctx.Err() != nil; done != tt.expectDone {
					t.Fatalf("after %s: context done = %v (err %v), want %v", tt.elapsed, done, ctx.Err(), tt.expectDone)
				}
			})
		})
	}
}
