package pgwriter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJobRunInsertSQLIdempotencyClauses(t *testing.T) {
	tests := []struct {
		name       string
		idempotent bool
		contains   bool
	}{
		{name: "batch insert has no conflict handling", idempotent: false, contains: false},
		{name: "single-run insert returns the ownership winner", idempotent: true, contains: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := jobRunInsertSQL(tc.idempotent)
			assert.Equal(t, 1, strings.Count(query, "INSERT INTO prow_job_runs"))
			assert.Equal(t, tc.contains, strings.Contains(query, "ON CONFLICT (id) DO NOTHING"))
			assert.Equal(t, tc.contains, strings.Contains(query, "RETURNING id"))
		})
	}
}
