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
		name, fields, expected string
	}{
		{"minimal", "id,description", `[{"id":42,"description":"test triage"}]`},
		{"duplicates and whitespace", "id,id, description,description", `[{"id":42,"description":"test triage"}]`},
		{"timestamp", "updated_at", `[{"updated_at":"2026-10-14T12:00:00Z"}]`},
		{"omitted nil", "bug,bug_id", `[{}]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := ParseFieldSelection(tc.fields, models.Triage{})
			require.NoError(t, err)
			response, err := fields.Project([]models.Triage{record})
			require.NoError(t, err)
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			require.JSONEq(t, tc.expected, string(encoded))
		})
	}
}

func TestParseFieldSelection(t *testing.T) {
	for _, raw := range []string{"missing", "ID", "triage_symptoms", "regressions.id", "links.self", "id,"} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseFieldSelection(raw, models.Triage{})
			require.Error(t, err)
			require.True(t, IsBadRequestError(err))
		})
	}
	fields, err := ParseFieldSelection(" id,description ", models.Triage{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"id", "description"}, fields.UnsortedList())
}

func TestFieldSelectionEmptyAndNull(t *testing.T) {
	for _, raw := range []string{"", " "} {
		fields, err := ParseFieldSelection(raw, models.Triage{})
		require.NoError(t, err)
		record := models.Triage{ID: 1}
		response, err := fields.Project(record)
		require.NoError(t, err)
		require.Equal(t, record, response)
	}
	fields, err := ParseFieldSelection("id,regressions", models.Triage{})
	require.NoError(t, err)
	for _, tc := range []struct {
		data any
		want string
	}{
		{[]models.Triage{}, `[]`},
		{[]models.Triage(nil), `null`},
		{models.Triage{ID: 1}, `{"id":1,"regressions":null}`},
		{models.Triage{ID: 1, Regressions: []models.TestRegression{}}, `{"id":1,"regressions":[]}`},
	} {
		response, err := fields.Project(tc.data)
		require.NoError(t, err)
		encoded, err := json.Marshal(response)
		require.NoError(t, err)
		require.JSONEq(t, tc.want, string(encoded))
	}
}

func TestFieldSelectionPreservesIntegerPrecision(t *testing.T) {
	model := struct {
		ID uint64 `json:"id"`
	}{ID: math.MaxUint64}
	fields, err := ParseFieldSelection("id", model)
	require.NoError(t, err)
	response, err := fields.Project(model)
	require.NoError(t, err)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	require.Equal(t, `{"id":18446744073709551615}`, string(encoded))
}

func TestFieldSelectionEncodingError(t *testing.T) {
	fields, err := ParseFieldSelection("id", models.Triage{})
	require.NoError(t, err)
	_, err = fields.Project(make(chan int))
	require.ErrorContains(t, err, "encoding fields response")
}
