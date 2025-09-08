package models

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestMarshalJSONForAudit(t *testing.T) {
	tests := []struct {
		name     string
		triage   Triage
		expected string
	}{
		{
			name: "basic triage with multiple regressions",
			triage: Triage{
				ID:          1,
				CreatedAt:   time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
				UpdatedAt:   time.Date(2025, 1, 2, 4, 5, 6, 0, time.UTC),
				URL:         "https://issues.redhat.com/browse/OCPBUGS-1234",
				Description: "Example bug",
				Type:        TriageType("infra"),
				Regressions: []TestRegression{
					{
						ID:          42,
						View:        "4.19-main",
						Release:     "4.19",
						TestID:      "some-test-id",
						TestName:    "TestSomethingCritical",
						Variants:    []string{"variant-a", "variant-b"},
						Opened:      time.Date(2022, 12, 15, 0, 0, 0, 0, time.UTC),
						Closed:      sql.NullTime{Time: time.Date(2023, 1, 5, 0, 0, 0, 0, time.UTC), Valid: true},
						LastFailure: sql.NullTime{Time: time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC), Valid: true},
						MaxFailures: 7,
					},
					{
						ID:          100,
						View:        "4.19-main",
						Release:     "4.19",
						TestID:      "some-test-other-id",
						TestName:    "TestSomethingCritical",
						Variants:    []string{"variant-a", "variant-b"},
						Opened:      time.Date(2022, 12, 15, 0, 0, 0, 0, time.UTC),
						Closed:      sql.NullTime{Time: time.Date(2023, 1, 5, 0, 0, 0, 0, time.UTC), Valid: true},
						LastFailure: sql.NullTime{Time: time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC), Valid: true},
						MaxFailures: 7,
					},
				},
			},
			expected: `{"id":1,"created_at":"2025-01-02T03:04:05Z","updated_at":"2025-01-02T04:05:06Z","url":"https://issues.redhat.com/browse/OCPBUGS-1234","description":"Example bug","type":"infra","resolved":{"Time":"0001-01-01T00:00:00Z","Valid":false},"resolution_reason":"","regressions":[{"id":42},{"id":100}]}`,
		},
		{
			name: "triage with no regressions",
			triage: Triage{
				ID:        2,
				CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 6, 1, 1, 0, 0, 0, time.UTC),
				URL:       "https://issues.redhat.com/browse/OCPBUGS-1234",
				Type:      TriageType("ci-infra"),
			},
			expected: `{"id":2,"created_at":"2025-06-01T00:00:00Z","updated_at":"2025-06-01T01:00:00Z","url":"https://issues.redhat.com/browse/OCPBUGS-1234","type":"ci-infra","resolved":{"Time":"0001-01-01T00:00:00Z","Valid":false},"resolution_reason":"","regressions":[]}`,
		},
		{
			name: "triage with bug populated, but bug should be omitted from audit json",
			triage: Triage{
				ID:        3,
				CreatedAt: time.Date(2023, 3, 1, 12, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2023, 3, 1, 13, 0, 0, 0, time.UTC),
				URL:       "https://issues.redhat.com/browse/OCPBUGS-1234",
				Type:      TriageType("test"),
				Bug: &Bug{
					ID:      99,
					Key:     "BUG-456",
					Status:  "OPEN",
					Summary: "This should be omitted",
					URL:     "https://issues.redhat.com/browse/OCPBUGS-1234",
				},
				BugID: ptrUint(99),
				Regressions: []TestRegression{
					{ID: 100},
				},
				Resolved: sql.NullTime{},
			},
			expected: `{"id":3,"created_at":"2023-03-01T12:00:00Z","updated_at":"2023-03-01T13:00:00Z","url":"https://issues.redhat.com/browse/OCPBUGS-1234","type":"test","bug_id":99,"resolved":{"Time":"0001-01-01T00:00:00Z","Valid":false},"resolution_reason":"","regressions":[{"id":100}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.triage.marshalJSONForAudit()
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}

			if diff := cmp.Diff(tt.expected, string(data)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func ptrUint(val uint) *uint {
	return &val
}
