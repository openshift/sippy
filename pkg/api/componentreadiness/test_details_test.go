package componentreadiness

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestMissingMultiTestStatusKeys(t *testing.T) {
	present := crtest.KeyWithVariants{TestID: "present", Variants: map[string]string{"Platform": "aws"}}
	missing := crtest.KeyWithVariants{TestID: "missing", Variants: map[string]string{"Platform": "gcp"}}

	missingKeys := missingMultiTestStatusKeys(crstatus.TestJobRunStatuses{
		SampleStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}},
		},
	}, []reqopts.TestIdentification{
		{TestID: present.TestID, RequestedVariants: present.Variants},
		{TestID: missing.TestID, RequestedVariants: missing.Variants},
	})

	assert.Equal(t, []string{missing.Encode()}, missingKeys)
}

func TestMultiTestStatusQueryBackoff(t *testing.T) {
	assert.Equal(t, wait.Backoff{
		Steps:    5,
		Duration: time.Minute,
		Factor:   2,
	}, multiTestStatusQueryBackoff)
}

func TestGetCompleteMultiTestJobRunStatuses(t *testing.T) {
	present := crtest.KeyWithVariants{TestID: "present", Variants: map[string]string{"Platform": "aws"}}
	missing := crtest.KeyWithVariants{TestID: "missing", Variants: map[string]string{"Platform": "gcp"}}
	fullStatuses := crstatus.TestJobRunStatuses{
		SampleStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}, {TestKeyStr: missing.Encode()}},
		},
	}
	partialStatuses := crstatus.TestJobRunStatuses{
		SampleStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}},
		},
	}
	baseStatuses := crstatus.TestJobRunStatuses{
		BaseStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}, {TestKeyStr: missing.Encode()}},
		},
	}
	baseOverrideStatuses := crstatus.TestJobRunStatuses{
		BaseOverrideStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}, {TestKeyStr: missing.Encode()}},
		},
	}

	originalBackoff := multiTestStatusQueryBackoff
	multiTestStatusQueryBackoff = wait.Backoff{Steps: 5}
	t.Cleanup(func() { multiTestStatusQueryBackoff = originalBackoff })

	for _, testCase := range []struct {
		name       string
		responses  []jobRunTestStatusResponse
		wantCalls  int
		wantErr    error
		wantResult crstatus.TestJobRunStatuses
	}{
		{
			name: "retries incomplete response until complete",
			responses: []jobRunTestStatusResponse{
				{statuses: partialStatuses},
				{statuses: fullStatuses},
			},
			wantCalls:  2,
			wantResult: fullStatuses,
		},
		{
			name: "reports incomplete response after exhausting retries",
			responses: []jobRunTestStatusResponse{
				{statuses: partialStatuses}, {statuses: partialStatuses}, {statuses: partialStatuses},
				{statuses: partialStatuses}, {statuses: partialStatuses},
			},
			wantCalls: 5,
			wantErr:   errors.New("test details status query remained incomplete after 5 attempts"),
		},
		{
			name:      "propagates query errors without retrying",
			responses: []jobRunTestStatusResponse{{errs: []error{errors.New("BigQuery unavailable")}}},
			wantCalls: 1,
			wantErr:   errors.New("BigQuery unavailable"),
		},
		{
			name:       "accepts key from base status",
			responses:  []jobRunTestStatusResponse{{statuses: baseStatuses}},
			wantCalls:  1,
			wantResult: baseStatuses,
		},
		{
			name:       "accepts key from base override status",
			responses:  []jobRunTestStatusResponse{{statuses: baseOverrideStatuses}},
			wantCalls:  1,
			wantResult: baseOverrideStatuses,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			generator := ComponentReportGenerator{
				ReqOptions: reqopts.RequestOptions{TestIDOptions: []reqopts.TestIdentification{
					{TestID: present.TestID, RequestedVariants: present.Variants},
					{TestID: missing.TestID, RequestedVariants: missing.Variants},
				}},
				jobRunTestStatusFetcher: func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error) {
					response := testCase.responses[calls]
					calls++
					return response.statuses, response.errs
				},
			}

			statuses, errs := generator.getCompleteMultiTestJobRunStatuses(context.Background())
			assert.Equal(t, testCase.wantCalls, calls)
			if testCase.wantErr != nil {
				require.Error(t, errors.Join(errs...))
				assert.Contains(t, errors.Join(errs...).Error(), testCase.wantErr.Error())
				return
			}
			require.Empty(t, errs)
			assert.Equal(t, testCase.wantResult, statuses)
		})
	}
}

func TestGetCompleteMultiTestJobRunStatusesWrapsWaitError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	generator := ComponentReportGenerator{
		jobRunTestStatusFetcher: func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error) {
			return crstatus.TestJobRunStatuses{}, nil
		},
	}
	_, errs := generator.getCompleteMultiTestJobRunStatuses(ctx)
	err := errors.Join(errs...)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "wait for complete multi-test job run statuses")
}

type jobRunTestStatusResponse struct {
	statuses crstatus.TestJobRunStatuses
	errs     []error
}
