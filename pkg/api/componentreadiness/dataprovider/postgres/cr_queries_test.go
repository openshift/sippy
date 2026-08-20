package postgres

import (
	"testing"

	"github.com/lib/pq"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestBuildDrilldownFilters(t *testing.T) {
	tests := []struct {
		name             string
		reqOptions       reqopts.RequestOptions
		wantInnerClause  string
		wantInnerArgs    []any
		wantOuterClause  string
		wantOuterArgLen  int
		wantCapabilities bool
	}{
		{
			name:            "empty options produce no filters",
			reqOptions:      reqopts.RequestOptions{},
			wantInnerClause: "",
			wantOuterClause: "",
		},
		{
			name: "capabilities filter generates array overlap clause",
			reqOptions: reqopts.RequestOptions{
				TestFilters: reqopts.TestFilters{
					Capabilities: []string{"install"},
				},
			},
			wantInnerClause:  "",
			wantOuterClause:  " AND tow.capabilities && ?",
			wantOuterArgLen:  1,
			wantCapabilities: true,
		},
		{
			name: "multiple capabilities generates single array overlap clause",
			reqOptions: reqopts.RequestOptions{
				TestFilters: reqopts.TestFilters{
					Capabilities: []string{"install", "networking"},
				},
			},
			wantInnerClause:  "",
			wantOuterClause:  " AND tow.capabilities && ?",
			wantOuterArgLen:  1,
			wantCapabilities: true,
		},
		{
			name: "test ID filter generates inner and outer clauses",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{
					{TestID: "test-unique-id"},
				},
			},
			wantInnerClause: " AND e.test_id IN (SELECT test_id FROM test_ownerships WHERE unique_id = ?)",
			wantInnerArgs:   []any{"test-unique-id"},
			wantOuterClause: " AND tow.unique_id = ?",
			wantOuterArgLen: 1,
		},
		{
			name: "per-test capability filter generates ANY clause",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{
					{Capability: "install"},
				},
			},
			wantInnerClause: "",
			wantOuterClause: " AND ? = ANY(tow.capabilities)",
			wantOuterArgLen: 1,
		},
		{
			name: "test ID with per-test capability and request-level capabilities",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{
					{TestID: "test-unique-id", Capability: "install"},
				},
				TestFilters: reqopts.TestFilters{
					Capabilities: []string{"install", "networking"},
				},
			},
			wantInnerClause:  " AND e.test_id IN (SELECT test_id FROM test_ownerships WHERE unique_id = ?)",
			wantInnerArgs:    []any{"test-unique-id"},
			wantOuterClause:  " AND tow.unique_id = ? AND ? = ANY(tow.capabilities) AND tow.capabilities && ?",
			wantOuterArgLen:  3,
			wantCapabilities: true,
		},
		{
			name: "multiple test ID options skips per-test filters but applies capabilities",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{
					{TestID: "test-1"},
					{TestID: "test-2"},
				},
				TestFilters: reqopts.TestFilters{
					Capabilities: []string{"install"},
				},
			},
			wantInnerClause:  "",
			wantOuterClause:  " AND tow.capabilities && ?",
			wantOuterArgLen:  1,
			wantCapabilities: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildDrilldownFilters(tc.reqOptions)

			if got.innerClause != tc.wantInnerClause {
				t.Errorf("innerClause = %q, want %q", got.innerClause, tc.wantInnerClause)
			}

			if tc.wantInnerArgs != nil {
				if len(got.innerArgs) != len(tc.wantInnerArgs) {
					t.Fatalf("innerArgs len = %d, want %d", len(got.innerArgs), len(tc.wantInnerArgs))
				}
				for i, want := range tc.wantInnerArgs {
					if got.innerArgs[i] != want {
						t.Errorf("innerArgs[%d] = %v, want %v", i, got.innerArgs[i], want)
					}
				}
			}

			if got.outerClause != tc.wantOuterClause {
				t.Errorf("outerClause = %q, want %q", got.outerClause, tc.wantOuterClause)
			}

			if len(got.outerArgs) != tc.wantOuterArgLen {
				t.Fatalf("outerArgs len = %d, want %d", len(got.outerArgs), tc.wantOuterArgLen)
			}

			if tc.wantCapabilities {
				lastArg := got.outerArgs[len(got.outerArgs)-1]
				if _, ok := lastArg.(*pq.StringArray); !ok {
					t.Errorf("last outerArg is %T, want *pq.StringArray (from pq.Array)", lastArg)
				}
			}
		})
	}
}
