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
		name                    string
		reqOptions              reqopts.RequestOptions
		wantOuterContains       []string
		wantOuterNotContains    string
		wantOuterArgs           []any
		wantInnerClauseNotEmpty bool
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
			wantOuterArgs:           []any{"test-123", "install"},
			wantInnerClauseNotEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := buildDrilldownFilters(tc.reqOptions)

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

			if tc.wantInnerClauseNotEmpty && f.innerClause == "" {
				t.Error("innerClause is empty, want non-empty")
			}
		})
	}
}
