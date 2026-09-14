package jobrunscan

import (
	"context"
	"testing"

	models "github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/stretchr/testify/require"
)

func TestReEvaluateOneFromCacheResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		loaded    bool
		symptoms  []models.Symptom
		want      *ReEvaluationResult
		wantError bool
	}{
		{name: "uninitialized", wantError: true},
		{name: "no active symptoms", loaded: true, want: &ReEvaluationResult{ProwJobBuildID: "invalid", Status: ReEvalSuccess}},
		{name: "evaluation failure", loaded: true, symptoms: []models.Symptom{{}}, wantError: true, want: &ReEvaluationResult{ProwJobBuildID: "invalid", Status: ReEvalEvalError, SymptomsEvaluated: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &ReEvaluator{symptoms: symptomCache{loaded: tc.loaded, symptoms: tc.symptoms}}
			result, err := r.ReEvaluateOneFromCache(context.Background(), "invalid", true)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.want == nil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			if tc.wantError {
				require.NotEmpty(t, result.Error)
				result.Error = ""
			}
			require.Equal(t, tc.want, result)
		})
	}
}
