package reqopts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAnyAreFiltered pins down which requests count as drilldowns. Callers with nothing to
// filter on still build a one element slice holding a zero value TestIdentification, so slice
// length alone cannot answer the question.
func TestAnyAreFiltered(t *testing.T) {
	tests := []struct {
		name string
		opts []TestIdentification
		want bool
	}{
		{
			name: "nil options are unfiltered",
			opts: nil,
		},
		{
			name: "empty slice is unfiltered",
			opts: []TestIdentification{},
		},
		{
			name: "one zero value identification is unfiltered",
			opts: []TestIdentification{{}},
		},
		{
			name: "an initialized but empty variants map is unfiltered",
			opts: []TestIdentification{{RequestedVariants: map[string]string{}}},
		},
		{
			name: "component drilldown is filtered",
			opts: []TestIdentification{{Component: "Networking"}},
			want: true,
		},
		{
			name: "capability drilldown is filtered",
			opts: []TestIdentification{{Component: "Networking", Capability: "Legacy"}},
			want: true,
		},
		{
			name: "exact test drilldown is filtered",
			opts: []TestIdentification{{TestID: "abc123"}},
			want: true,
		},
		{
			name: "requested variants alone are a filter",
			opts: []TestIdentification{{RequestedVariants: map[string]string{"Platform": "aws"}}},
			want: true,
		},
		{
			name: "base override release alone is a filter",
			opts: []TestIdentification{{BaseOverrideRelease: "4.14"}},
			want: true,
		},
		{
			name: "a filtered entry among empty ones still counts",
			opts: []TestIdentification{{}, {TestID: "abc123"}},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, AnyAreFiltered(tc.opts))
		})
	}
}
