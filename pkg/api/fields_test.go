package api

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/require"
)

func TestFieldSelection(t *testing.T) {
	record := models.Triage{
		ID: 42, URL: "https://example.com/bug", Description: "test triage",
		UpdatedAt:   time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC),
		Regressions: []models.TestRegression{{ID: 7, TestName: "large details"}},
	}
	tests := []struct {
		name         string
		fields       string
		data         any
		expected     string
		wantOriginal bool
		wantError    string
	}{
		{
			name: "minimal", fields: "id,description", data: []models.Triage{record},
			expected: `[{"description":"test triage","id":42}]`,
		},
		{
			name: "duplicates and whitespace", fields: "id,id, description,description", data: []models.Triage{record},
			expected: `[{"description":"test triage","id":42}]`,
		},
		{
			name: "timestamp", fields: "updated_at", data: []models.Triage{record},
			expected: `[{"updated_at":"2026-10-14T12:00:00Z"}]`,
		},
		{
			name: "omitted nil", fields: "bug,bug_id", data: []models.Triage{record},
			expected: `[{}]`,
		},
		{
			name: "empty selection preserves original", data: record, wantOriginal: true,
		},
		{
			name: "whitespace selection preserves original", fields: " ", data: record, wantOriginal: true,
		},
		{
			name: "empty array", fields: "id,regressions", data: []models.Triage{}, expected: `[]`,
		},
		{
			name: "nil array", fields: "id,regressions", data: []models.Triage(nil), expected: `null`,
		},
		{
			name: "nil regressions", fields: "id,regressions", data: models.Triage{ID: 1},
			expected: `{"id":1,"regressions":null}`,
		},
		{
			name: "empty regressions", fields: "id,regressions", data: models.Triage{ID: 1, Regressions: []models.TestRegression{}},
			expected: `{"id":1,"regressions":[]}`,
		},
		{
			name: "integer precision", fields: "id",
			data: struct {
				ID uint64 `json:"id"`
			}{ID: math.MaxUint64},
			expected: `{"id":18446744073709551615}`,
		},
		{
			name: "encoding error", fields: "id", data: make(chan int), wantError: "encoding fields response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := ParseFieldSelection(tc.fields, models.Triage{})
			require.NoError(t, err)
			response, err := fields.Project(tc.data)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			if tc.wantOriginal {
				require.Equal(t, tc.data, response)
				return
			}
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			require.Equal(t, tc.expected, string(encoded))
		})
	}
}

func TestParseFieldSelection(t *testing.T) {
	tests := []struct {
		name      string
		fields    string
		expected  []string
		wantError bool
	}{
		{name: "empty", fields: ""},
		{name: "whitespace only", fields: " "},
		{name: "valid fields with whitespace", fields: " id,description ", expected: []string{"id", "description"}},
		{name: "duplicate fields", fields: "id,id", expected: []string{"id"}},
		{name: "unknown field", fields: "missing", wantError: true},
		{name: "Go field name", fields: "ID", wantError: true},
		{name: "nested slice field", fields: "regressions.id", wantError: true},
		{name: "nested map field", fields: "links.self", wantError: true},
		{name: "trailing comma", fields: "id,", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := ParseFieldSelection(tc.fields, models.Triage{})
			if tc.wantError {
				require.Error(t, err)
				require.True(t, IsBadRequestError(err))
				return
			}
			require.NoError(t, err)
			require.ElementsMatch(t, tc.expected, fields.UnsortedList())
		})
	}
}
