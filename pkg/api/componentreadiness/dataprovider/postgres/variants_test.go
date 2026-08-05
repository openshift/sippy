package postgres

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestBuildVariantFilterClause(t *testing.T) {
	tests := []struct {
		name            string
		includeVariants map[string][]string
		wantClause      string
		wantArgCount    int
		wantArgs        []any
	}{
		{
			name:            "empty filter",
			includeVariants: map[string][]string{},
			wantClause:      "",
			wantArgCount:    0,
		},
		{
			name:            "single key single value",
			includeVariants: map[string][]string{"Platform": {"aws"}},
			wantClause:      "variants && ARRAY[?]::text[]",
			wantArgCount:    1,
			wantArgs:        []any{"Platform:aws"},
		},
		{
			name:            "single key multiple values",
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			wantClause:      "variants && ARRAY[?, ?]::text[]",
			wantArgCount:    2,
			wantArgs:        []any{"Platform:aws", "Platform:gcp"},
		},
		{
			name: "multiple keys sorted alphabetically",
			includeVariants: map[string][]string{
				"Platform": {"aws"},
				"Network":  {"ovn"},
			},
			wantClause:   "variants && ARRAY[?]::text[] AND variants && ARRAY[?]::text[]",
			wantArgCount: 2,
			wantArgs:     []any{"Network:ovn", "Platform:aws"},
		},
		{
			name:            "key with empty values skipped",
			includeVariants: map[string][]string{"Platform": {}},
			wantClause:      "",
			wantArgCount:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := buildVariantFilterClause(tc.includeVariants)
			if clause != tc.wantClause {
				t.Errorf("clause = %q, want %q", clause, tc.wantClause)
			}
			if len(args) != tc.wantArgCount {
				t.Errorf("len(args) = %d, want %d", len(args), tc.wantArgCount)
			}
			if tc.wantArgs != nil {
				for i, want := range tc.wantArgs {
					if i >= len(args) {
						break
					}
					if args[i] != want {
						t.Errorf("args[%d] = %v, want %v", i, args[i], want)
					}
				}
			}
		})
	}
}

func TestMergeRequestedVariants(t *testing.T) {
	tests := []struct {
		name            string
		includeVariants map[string][]string
		reqOptions      reqopts.RequestOptions
		want            map[string][]string
	}{
		{
			name:            "no TestIDOptions returns input unchanged",
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			reqOptions:      reqopts.RequestOptions{},
			want:            map[string][]string{"Platform": {"aws", "gcp"}},
		},
		{
			name:            "multiple TestIDOptions returns input unchanged",
			includeVariants: map[string][]string{"Platform": {"aws"}},
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{}, {}},
			},
			want: map[string][]string{"Platform": {"aws"}},
		},
		{
			name:            "empty RequestedVariants returns input unchanged",
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{}},
			},
			want: map[string][]string{"Platform": {"aws", "gcp"}},
		},
		{
			name:            "overrides existing key with single value",
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}, "Network": {"ovn", "sdn"}},
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{
					RequestedVariants: map[string]string{"Platform": "aws"},
				}},
			},
			want: map[string][]string{"Platform": {"aws"}, "Network": {"ovn", "sdn"}},
		},
		{
			name:            "adds new key",
			includeVariants: map[string][]string{"Platform": {"aws"}},
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{
					RequestedVariants: map[string]string{"Topology": "ha"},
				}},
			},
			want: map[string][]string{"Platform": {"aws"}, "Topology": {"ha"}},
		},
		{
			name:            "does not modify original map",
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{
					RequestedVariants: map[string]string{"Platform": "aws"},
				}},
			},
			want: map[string][]string{"Platform": {"aws"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origLen := len(tc.includeVariants["Platform"])
			result := mergeRequestedVariants(tc.includeVariants, tc.reqOptions)

			for k, wantVals := range tc.want {
				gotVals, ok := result[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if len(gotVals) != len(wantVals) {
					t.Errorf("key %q: got %v, want %v", k, gotVals, wantVals)
					continue
				}
				for i, want := range wantVals {
					if gotVals[i] != want {
						t.Errorf("key %q[%d] = %q, want %q", k, i, gotVals[i], want)
					}
				}
			}

			if tc.name == "does not modify original map" {
				if len(tc.includeVariants["Platform"]) != origLen {
					t.Error("original includeVariants was mutated")
				}
			}
		})
	}
}

func TestFilterVariantsByDBGroupBy(t *testing.T) {
	tests := []struct {
		name            string
		includeVariants map[string][]string
		dbGroupBy       sets.Set[string]
		want            map[string][]string
	}{
		{
			name:            "empty inputs",
			includeVariants: map[string][]string{},
			dbGroupBy:       sets.New[string](),
			want:            map[string][]string{},
		},
		{
			name: "keeps only dbGroupBy keys",
			includeVariants: map[string][]string{
				"Architecture": {"amd64"},
				"JobTier":      {"blocking", "informing"},
				"Platform":     {"aws"},
				"Owner":        {"eng"},
			},
			dbGroupBy: sets.New[string]("Architecture", "Platform"),
			want: map[string][]string{
				"Architecture": {"amd64"},
				"Platform":     {"aws"},
			},
		},
		{
			name: "all keys in dbGroupBy preserved",
			includeVariants: map[string][]string{
				"Architecture": {"amd64"},
				"Network":      {"ovn"},
			},
			dbGroupBy: sets.New[string]("Architecture", "Network", "Platform"),
			want: map[string][]string{
				"Architecture": {"amd64"},
				"Network":      {"ovn"},
			},
		},
		{
			name: "no keys in dbGroupBy",
			includeVariants: map[string][]string{
				"JobTier": {"blocking"},
				"Owner":   {"eng"},
			},
			dbGroupBy: sets.New[string]("Architecture", "Platform"),
			want:      map[string][]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := filterVariantsByDBGroupBy(tc.includeVariants, tc.dbGroupBy)
			if len(result) != len(tc.want) {
				t.Fatalf("got %d keys, want %d", len(result), len(tc.want))
			}
			for k, wantVals := range tc.want {
				gotVals, ok := result[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if len(gotVals) != len(wantVals) {
					t.Errorf("key %q: got %v, want %v", k, gotVals, wantVals)
					continue
				}
				for i, wantVal := range wantVals {
					if gotVals[i] != wantVal {
						t.Errorf("key %q[%d] = %q, want %q", k, i, gotVals[i], wantVal)
					}
				}
			}
		})
	}
}

func TestBuildVariantGroupMapping(t *testing.T) {
	tests := []struct {
		name              string
		variantLookup     map[uint]map[string]string
		wantValuesClause  string
		wantGroupVariants map[int]map[string]string
	}{
		{
			name:              "empty input",
			variantLookup:     map[uint]map[string]string{},
			wantValuesClause:  "",
			wantGroupVariants: map[int]map[string]string{},
		},
		{
			name: "single vcid",
			variantLookup: map[uint]map[string]string{
				1: {"Platform": "aws"},
			},
			wantValuesClause:  "VALUES (1,0)",
			wantGroupVariants: map[int]map[string]string{0: {"Platform": "aws"}},
		},
		{
			name: "two vcids same dimensions get same group",
			variantLookup: map[uint]map[string]string{
				1: {"Platform": "aws"},
				2: {"Platform": "aws"},
			},
			wantValuesClause:  "VALUES (1,0),(2,0)",
			wantGroupVariants: map[int]map[string]string{0: {"Platform": "aws"}},
		},
		{
			name: "two vcids different dimensions get different groups",
			variantLookup: map[uint]map[string]string{
				1: {"Platform": "aws"},
				2: {"Platform": "gcp"},
			},
			wantValuesClause: "VALUES (1,0),(2,1)",
			wantGroupVariants: map[int]map[string]string{
				0: {"Platform": "aws"},
				1: {"Platform": "gcp"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := buildVariantGroupMapping(tc.variantLookup)
			if result.valuesClause != tc.wantValuesClause {
				t.Errorf("valuesClause = %q, want %q", result.valuesClause, tc.wantValuesClause)
			}
			if len(result.groupToVariants) != len(tc.wantGroupVariants) {
				t.Fatalf("group count = %d, want %d", len(result.groupToVariants), len(tc.wantGroupVariants))
			}
			for gid, wantVars := range tc.wantGroupVariants {
				gotVars, ok := result.groupToVariants[gid]
				if !ok {
					t.Errorf("missing group %d", gid)
					continue
				}
				for k, wantV := range wantVars {
					if gotV := gotVars[k]; gotV != wantV {
						t.Errorf("group %d: got[%q] = %q, want %q", gid, k, gotV, wantV)
					}
				}
			}
		})
	}
}

func TestBuildColumnGroupMapping(t *testing.T) {
	tests := []struct {
		name             string
		groupToVariants  map[int]map[string]string
		columnGroupBy    sets.Set[string]
		wantValuesClause string
		wantNewEntries   map[int]map[string]string
	}{
		{
			name:             "empty input",
			groupToVariants:  map[int]map[string]string{},
			columnGroupBy:    sets.New[string]("Platform"),
			wantValuesClause: "",
			wantNewEntries:   map[int]map[string]string{},
		},
		{
			name: "groups with same column projection collapse",
			groupToVariants: map[int]map[string]string{
				0: {"Platform": "aws", "Network": "ovn"},
				1: {"Platform": "aws", "Network": "sdn"},
			},
			columnGroupBy:    sets.New[string]("Platform"),
			wantValuesClause: "VALUES (0,2),(1,2)",
			wantNewEntries:   map[int]map[string]string{2: {"Platform": "aws"}},
		},
		{
			name: "groups with different column projection stay separate",
			groupToVariants: map[int]map[string]string{
				0: {"Platform": "aws", "Network": "ovn"},
				1: {"Platform": "gcp", "Network": "ovn"},
			},
			columnGroupBy:    sets.New[string]("Platform"),
			wantValuesClause: "VALUES (0,2),(1,3)",
			wantNewEntries: map[int]map[string]string{
				2: {"Platform": "aws"},
				3: {"Platform": "gcp"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			initialSize := len(tc.groupToVariants)
			result := buildColumnGroupMapping(tc.groupToVariants, tc.columnGroupBy)

			if result.valuesClause != tc.wantValuesClause {
				t.Errorf("valuesClause = %q, want %q", result.valuesClause, tc.wantValuesClause)
			}

			newEntryCount := len(tc.groupToVariants) - initialSize
			if newEntryCount != len(tc.wantNewEntries) {
				t.Fatalf("new synthetic entries = %d, want %d", newEntryCount, len(tc.wantNewEntries))
			}
			for gid, wantVars := range tc.wantNewEntries {
				gotVars, ok := tc.groupToVariants[gid]
				if !ok {
					t.Errorf("missing synthetic group %d", gid)
					continue
				}
				for k, wantV := range wantVars {
					if gotV := gotVars[k]; gotV != wantV {
						t.Errorf("group %d: got[%q] = %q, want %q", gid, k, gotV, wantV)
					}
				}
			}
		})
	}
}
