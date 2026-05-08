package bugloader

import (
	"testing"

	"github.com/openshift/sippy/pkg/db/models"
)

func TestSkipTriageBugLoaderPass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name              string
		resolved          bool
		bugLinked         bool
		triageDescription string
		bugSummary        string
		wantSkip          bool
	}{
		{
			name:              "resolved linked matching skips",
			resolved:          true,
			bugLinked:         true,
			triageDescription: "same",
			bugSummary:        "same",
			wantSkip:          true,
		},
		{
			name:              "resolved linked mismatch does not skip",
			resolved:          true,
			bugLinked:         true,
			triageDescription: "old",
			bugSummary:        "new",
			wantSkip:          false,
		},
		{
			name:              "not resolved never skips",
			resolved:          false,
			bugLinked:         true,
			triageDescription: "same",
			bugSummary:        "same",
			wantSkip:          false,
		},
		{
			name:              "not linked never skips",
			resolved:          true,
			bugLinked:         false,
			triageDescription: "same",
			bugSummary:        "same",
			wantSkip:          false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := skipTriageBugLoaderPass(tc.resolved, tc.bugLinked, tc.triageDescription, tc.bugSummary)
			if got != tc.wantSkip {
				t.Fatalf("skipTriageBugLoaderPass(...) = %v, want %v", got, tc.wantSkip)
			}
		})
	}
}

func TestApplyBugSummaryToTriageDescription(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		triageDesc    string
		bugSummary    string
		wantChanged   bool
		wantFinalDesc string
	}{
		{
			name:          "updates when summary differs",
			triageDesc:    "old title",
			bugSummary:    "new title",
			wantChanged:   true,
			wantFinalDesc: "new title",
		},
		{
			name:          "no change when already matches",
			triageDesc:    "same",
			bugSummary:    "same",
			wantChanged:   false,
			wantFinalDesc: "same",
		},
		{
			name:          "no change when summary empty",
			triageDesc:    "user text",
			bugSummary:    "",
			wantChanged:   false,
			wantFinalDesc: "user text",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := models.Triage{Description: tc.triageDesc}
			changed := applyBugSummaryToTriageDescription(&tr, tc.bugSummary)
			if changed != tc.wantChanged {
				t.Fatalf("applyBugSummaryToTriageDescription changed=%v, want %v", changed, tc.wantChanged)
			}
			if tr.Description != tc.wantFinalDesc {
				t.Fatalf("description = %q, want %q", tr.Description, tc.wantFinalDesc)
			}
		})
	}
}

func TestTriageBugLinked(t *testing.T) {
	t.Parallel()
	id := uint(1)
	url := "https://example/browse/X-1"
	bug := &models.Bug{ID: id, URL: url}

	t.Run("linked when ids and urls align", func(t *testing.T) {
		t.Parallel()
		tr := models.Triage{URL: url, BugID: &id, Bug: bug}
		if !triageBugLinked(&tr) {
			t.Fatal("expected linked")
		}
	})

	t.Run("not linked when bug association missing", func(t *testing.T) {
		t.Parallel()
		tr := models.Triage{URL: url, BugID: &id, Bug: nil}
		if triageBugLinked(&tr) {
			t.Fatal("expected not linked")
		}
	})

	t.Run("not linked when url mismatches bug url", func(t *testing.T) {
		t.Parallel()
		tr := models.Triage{URL: "https://other", BugID: &id, Bug: bug}
		if triageBugLinked(&tr) {
			t.Fatal("expected not linked")
		}
	})
}
