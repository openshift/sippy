package postgres

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestBuildDrilldownFilters(t *testing.T) {
	tests := []struct {
		name                 string
		reqOptions           reqopts.RequestOptions
		wantOuterContains    []string
		wantOuterNotContains string
		wantOuterArgs        []any
		wantInnerContains    []string
		wantInnerArgs        []any
	}{
		{
			name:          "empty options produces no clauses",
			reqOptions:    reqopts.RequestOptions{},
			wantOuterArgs: nil,
		},
		{
			name: "IgnoreDisruption adds disruption exclusion",
			reqOptions: reqopts.RequestOptions{
				AdvancedOption: reqopts.Advanced{
					IgnoreDisruption: true,
				},
			},
			wantOuterContains: []string{"AND NOT ('Disruption' = ANY(tow.capabilities))"},
			wantOuterArgs:     nil,
		},
		{
			name: "IgnoreDisruption false does not add disruption exclusion",
			reqOptions: reqopts.RequestOptions{
				AdvancedOption: reqopts.Advanced{
					IgnoreDisruption: false,
				},
			},
			wantOuterNotContains: "Disruption",
			wantOuterArgs:        nil,
		},
		{
			name: "capabilities filter and IgnoreDisruption combined",
			reqOptions: reqopts.RequestOptions{
				TestFilters: reqopts.TestFilters{
					Capabilities: []string{"install"},
				},
				AdvancedOption: reqopts.Advanced{
					IgnoreDisruption: true,
				},
			},
			wantOuterContains: []string{
				"AND tow.capabilities && ?",
				"AND NOT ('Disruption' = ANY(tow.capabilities))",
			},
			wantOuterArgs: []any{pq.Array([]string{"install"})},
		},
		{
			name: "single TestIDOption with test ID and capability",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{
					{TestID: "test-123", Capability: "install"},
				},
			},
			wantOuterContains: []string{
				"AND tow.unique_id = ?",
				"AND ? = ANY(tow.capabilities)",
			},
			wantOuterArgs: []any{"test-123", "install"},
			wantInnerContains: []string{
				"AND e.test_id IN (SELECT test_id FROM test_ownerships WHERE unique_id = ?)",
			},
			wantInnerArgs: []any{"test-123"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := buildDrilldownFilters(tc.reqOptions)

			if len(tc.wantOuterContains) == 0 && tc.wantOuterArgs == nil && f.outerClause != "" {
				t.Errorf("outerClause = %q, want empty", f.outerClause)
			}
			for _, want := range tc.wantOuterContains {
				if !strings.Contains(f.outerClause, want) {
					t.Errorf("outerClause = %q, want it to contain %q", f.outerClause, want)
				}
			}

			if tc.wantOuterNotContains != "" && strings.Contains(f.outerClause, tc.wantOuterNotContains) {
				t.Errorf("outerClause = %q, want it to NOT contain %q", f.outerClause, tc.wantOuterNotContains)
			}

			if len(f.outerArgs) != len(tc.wantOuterArgs) {
				t.Errorf("outerArgs count = %d, want %d", len(f.outerArgs), len(tc.wantOuterArgs))
			} else {
				for i := range tc.wantOuterArgs {
					if fmt.Sprintf("%v", f.outerArgs[i]) != fmt.Sprintf("%v", tc.wantOuterArgs[i]) {
						t.Errorf("outerArgs[%d] = %v, want %v", i, f.outerArgs[i], tc.wantOuterArgs[i])
					}
				}
			}

			if len(tc.wantInnerContains) == 0 && tc.wantInnerArgs == nil && f.innerClause != "" {
				t.Errorf("innerClause = %q, want empty", f.innerClause)
			}
			for _, want := range tc.wantInnerContains {
				if !strings.Contains(f.innerClause, want) {
					t.Errorf("innerClause = %q, want it to contain %q", f.innerClause, want)
				}
			}

			if len(f.innerArgs) != len(tc.wantInnerArgs) {
				t.Errorf("innerArgs count = %d, want %d", len(f.innerArgs), len(tc.wantInnerArgs))
			} else {
				for i := range tc.wantInnerArgs {
					if fmt.Sprintf("%v", f.innerArgs[i]) != fmt.Sprintf("%v", tc.wantInnerArgs[i]) {
						t.Errorf("innerArgs[%d] = %v, want %v", i, f.innerArgs[i], tc.wantInnerArgs[i])
					}
				}
			}
		})
	}
}
