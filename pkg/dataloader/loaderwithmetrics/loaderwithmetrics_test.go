package loaderwithmetrics

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openshift/sippy/pkg/dataloader"
)

// mockDataLoader implements the DataLoader interface for testing
type mockDataLoader struct {
	name string
}

func (m *mockDataLoader) Name() string {
	return m.name
}

func (m *mockDataLoader) Load() {
	// no-op for testing
}

func (m *mockDataLoader) Errors() []error {
	return nil
}

func TestSortLoaders(t *testing.T) {
	tests := []struct {
		name          string
		inputLoaders  []string
		expectedOrder []string
	}{
		{
			name:          "empty loaders",
			inputLoaders:  []string{},
			expectedOrder: []string{},
		},
		{
			name:          "single loader",
			inputLoaders:  []string{"prow"},
			expectedOrder: []string{"prow"},
		},
		{
			name:          "all loaders in correct order",
			inputLoaders:  []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates", "regression-cache"},
			expectedOrder: []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates", "regression-cache"},
		},
		{
			name:          "all loaders in reverse order",
			inputLoaders:  []string{"regression-cache", "feature-gates", "test-mapping", "bugs", "github", "jira", "releases", "prow"},
			expectedOrder: []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates", "regression-cache"},
		},
		{
			name:          "random order subset",
			inputLoaders:  []string{"bugs", "prow", "jira", "releases"},
			expectedOrder: []string{"prow", "releases", "jira", "bugs"},
		},
		{
			name:          "regression-cache runs last among known loaders",
			inputLoaders:  []string{"regression-cache", "prow"},
			expectedOrder: []string{"prow", "regression-cache"},
		},
		{
			name:          "unknown loaders with known ones",
			inputLoaders:  []string{"unknown-loader", "prow", "another-unknown", "releases"},
			expectedOrder: []string{"prow", "releases", "unknown-loader", "another-unknown"},
		},
		{
			name:          "only unknown loaders",
			inputLoaders:  []string{"unknown-1", "unknown-2", "unknown-3"},
			expectedOrder: []string{"unknown-1", "unknown-2", "unknown-3"},
		},
		{
			name:          "mixed known and unknown in random order",
			inputLoaders:  []string{"unknown-z", "github", "unknown-a", "prow", "unknown-m"},
			expectedOrder: []string{"prow", "github", "unknown-z", "unknown-a", "unknown-m"},
		},
		{
			name:          "duplicate loaders",
			inputLoaders:  []string{"prow", "prow", "releases", "releases"},
			expectedOrder: []string{"prow", "prow", "releases", "releases"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLoaders := make([]dataloader.DataLoader, len(tt.inputLoaders))
			for i, name := range tt.inputLoaders {
				mockLoaders[i] = &mockDataLoader{name: name}
			}

			loader := &LoaderWithMetrics{
				loaders: mockLoaders,
			}

			loader.sortLoaders()
			names := make([]string, len(loader.loaders))
			for i, l := range loader.loaders {
				names[i] = l.Name()
			}

			if diff := cmp.Diff(names, tt.expectedOrder); diff != "" {
				t.Errorf("loaders mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// recordingLoader exposes loader execution for ordering and failure gating.
type recordingLoader struct {
	name  string
	calls *[]string
	errs  []error
}

func (l *recordingLoader) Name() string    { return l.name }
func (l *recordingLoader) Load()           { *l.calls = append(*l.calls, l.name) }
func (l *recordingLoader) Errors() []error { return l.errs }

func TestGARefreshBeforeRegressionCache(t *testing.T) {
	for _, tc := range []struct {
		name     string
		gaErrors []error
		want     []string
	}{
		{"successful refresh", nil, []string{"ga-test-status", "regression-cache"}},
		{"failed refresh prevents publishing", []error{fmt.Errorf("refresh failed")}, []string{"ga-test-status"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			l := New([]dataloader.DataLoader{
				&recordingLoader{name: "regression-cache", calls: &calls},
				&recordingLoader{name: "ga-test-status", calls: &calls, errs: tc.gaErrors},
			})
			l.Load()
			if diff := cmp.Diff(tc.want, calls); diff != "" {
				t.Errorf("unexpected execution (-want +got): %s", diff)
			}
		})
	}
}
