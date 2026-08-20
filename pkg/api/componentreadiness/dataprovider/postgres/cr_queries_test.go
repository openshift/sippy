package postgres

import (
	"strings"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestBuildDrilldownFilters(t *testing.T) {
	tests := []struct {
		name                    string
		reqOptions              reqopts.RequestOptions
		wantOuterContains       string
		wantOuterNotContains    string
		wantOuterArgsCount      int
		wantInnerClauseNotEmpty bool
	}{
		{
			name:               "empty options produces no clauses",
			reqOptions:         reqopts.RequestOptions{},
			wantOuterArgsCount: 0,
		},
		{
			name: "IgnoreDisruption adds disruption exclusion",
			reqOptions: reqopts.RequestOptions{
				AdvancedOption: reqopts.Advanced{
					IgnoreDisruption: true,
				},
			},
			wantOuterContains:  "AND NOT ('Disruption' = ANY(tow.capabilities))",
			wantOuterArgsCount: 0,
		},
		{
			name: "IgnoreDisruption false does not add disruption exclusion",
			reqOptions: reqopts.RequestOptions{
				AdvancedOption: reqopts.Advanced{
					IgnoreDisruption: false,
				},
			},
			wantOuterNotContains: "Disruption",
			wantOuterArgsCount:   0,
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
			wantOuterContains:  "AND NOT ('Disruption' = ANY(tow.capabilities))",
			wantOuterArgsCount: 1,
		},
		{
			name: "single TestIDOption with test ID and capability",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{
					{TestID: "test-123", Capability: "install"},
				},
			},
			wantOuterContains:       "AND tow.unique_id = ?",
			wantOuterArgsCount:      2,
			wantInnerClauseNotEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := buildDrilldownFilters(tc.reqOptions)

			if tc.wantOuterContains != "" && !strings.Contains(f.outerClause, tc.wantOuterContains) {
				t.Errorf("outerClause = %q, want it to contain %q", f.outerClause, tc.wantOuterContains)
			}

			if tc.wantOuterNotContains != "" && strings.Contains(f.outerClause, tc.wantOuterNotContains) {
				t.Errorf("outerClause = %q, want it to NOT contain %q", f.outerClause, tc.wantOuterNotContains)
			}

			if len(f.outerArgs) != tc.wantOuterArgsCount {
				t.Errorf("outerArgs count = %d, want %d", len(f.outerArgs), tc.wantOuterArgsCount)
			}

			if tc.wantInnerClauseNotEmpty && f.innerClause == "" {
				t.Error("innerClause is empty, want non-empty")
			}
		})
	}
}
