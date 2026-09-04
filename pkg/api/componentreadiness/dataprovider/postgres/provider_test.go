package postgres

import (
	"strings"
	"testing"

	"github.com/lib/pq"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestAppendTestDetailsLifecycleFilter(t *testing.T) {
	tests := []struct {
		name           string
		lifecycles     []string
		wantFilter     bool
		wantAdditional int
	}{
		{
			name:           "configured lifecycle is filtered",
			lifecycles:     []string{"blocking"},
			wantFilter:     true,
			wantAdditional: 1,
		},
		{
			name:       "empty lifecycle is unfiltered",
			lifecycles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := []any{"existing"}
			query, args := appendTestDetailsLifecycleFilter("SELECT 1 WHERE true", originalArgs, tt.lifecycles)

			if got := strings.Contains(query, "pjrt.lifecycle = ANY(?)"); got != tt.wantFilter {
				t.Fatalf("lifecycle filter present = %t, want %t", got, tt.wantFilter)
			}
			if got, want := len(args), len(originalArgs)+tt.wantAdditional; got != want {
				t.Fatalf("argument count = %d, want %d", got, want)
			}
			if tt.wantAdditional == 1 {
				got, ok := args[len(args)-1].(*pq.StringArray)
				if !ok {
					t.Fatalf("lifecycle argument has type %T, want *pq.StringArray", args[len(args)-1])
				}
				if strings.Join(*got, ",") != strings.Join(tt.lifecycles, ",") {
					t.Fatalf("lifecycle argument = %v, want %v", got, tt.lifecycles)
				}
			}
		})
	}
}

func TestParseVariants(t *testing.T) {
	tests := []struct {
		name     string
		variants pq.StringArray
		want     map[string]string
	}{
		{
			name:     "empty array",
			variants: pq.StringArray{},
			want:     map[string]string{},
		},
		{
			name:     "single variant",
			variants: pq.StringArray{"Platform:aws"},
			want:     map[string]string{"Platform": "aws"},
		},
		{
			name:     "multiple variants",
			variants: pq.StringArray{"Platform:aws", "Network:ovn", "Architecture:amd64"},
			want:     map[string]string{"Platform": "aws", "Network": "ovn", "Architecture": "amd64"},
		},
		{
			name:     "value containing colon uses first colon only",
			variants: pq.StringArray{"Suite:openshift-tests:sig-auth"},
			want:     map[string]string{"Suite": "openshift-tests:sig-auth"},
		},
		{
			name:     "entry without colon is skipped",
			variants: pq.StringArray{"malformed", "Platform:aws"},
			want:     map[string]string{"Platform": "aws"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVariants(tc.variants)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for k, wantV := range tc.want {
				if gotV, ok := got[k]; !ok {
					t.Errorf("missing key %q", k)
				} else if gotV != wantV {
					t.Errorf("got[%q] = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestFilterByDBGroupBy(t *testing.T) {
	tests := []struct {
		name      string
		variants  map[string]string
		dbGroupBy sets.Set[string]
		want      map[string]string
	}{
		{
			name:      "empty variants",
			variants:  map[string]string{},
			dbGroupBy: sets.New[string]("Platform"),
			want:      map[string]string{},
		},
		{
			name:      "keeps matching keys",
			variants:  map[string]string{"Platform": "aws", "Network": "ovn", "Architecture": "amd64"},
			dbGroupBy: sets.New[string]("Platform", "Network"),
			want:      map[string]string{"Platform": "aws", "Network": "ovn"},
		},
		{
			name:      "no matching keys",
			variants:  map[string]string{"Platform": "aws"},
			dbGroupBy: sets.New[string]("Network"),
			want:      map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterByDBGroupBy(tc.variants, tc.dbGroupBy)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for k, wantV := range tc.want {
				if gotV := got[k]; gotV != wantV {
					t.Errorf("got[%q] = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestMatchesIncludeVariants(t *testing.T) {
	tests := []struct {
		name            string
		variants        map[string]string
		includeVariants map[string][]string
		want            bool
	}{
		{
			name:            "empty filter matches everything",
			variants:        map[string]string{"Platform": "aws"},
			includeVariants: map[string][]string{},
			want:            true,
		},
		{
			name:            "matching single key",
			variants:        map[string]string{"Platform": "aws", "Network": "ovn"},
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			want:            true,
		},
		{
			name:            "value not in allowed list",
			variants:        map[string]string{"Platform": "azure"},
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			want:            false,
		},
		{
			name:            "required key missing from variants",
			variants:        map[string]string{"Platform": "aws"},
			includeVariants: map[string][]string{"Network": {"ovn"}},
			want:            false,
		},
		{
			name:            "all keys must match",
			variants:        map[string]string{"Platform": "aws", "Network": "sdn"},
			includeVariants: map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			want:            false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesIncludeVariants(tc.variants, tc.includeVariants)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
