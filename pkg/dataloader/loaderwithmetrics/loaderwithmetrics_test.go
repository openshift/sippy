package loaderwithmetrics

import (
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
			inputLoaders:  []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates", "component-readiness-cache", "regression-tracker"},
			expectedOrder: []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates", "component-readiness-cache", "regression-tracker"},
		},
		{
			name:          "all loaders in reverse order",
			inputLoaders:  []string{"regression-tracker", "component-readiness-cache", "feature-gates", "test-mapping", "bugs", "github", "jira", "releases", "prow"},
			expectedOrder: []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates", "component-readiness-cache", "regression-tracker"},
		},
		{
			name:          "random order subset",
			inputLoaders:  []string{"bugs", "prow", "jira", "releases"},
			expectedOrder: []string{"prow", "releases", "jira", "bugs"},
		},
		{
			name:          "component-readiness-cache before regression-tracker",
			inputLoaders:  []string{"regression-tracker", "component-readiness-cache"},
			expectedOrder: []string{"component-readiness-cache", "regression-tracker"},
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
