package api

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/filter"
)

func TestApplyJobRunFilters(t *testing.T) {
	lookback := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		filter           *filter.Filter
		wantWhere        string
		wantVars         []any
		wantValidation   bool
		wantInvalidField bool
	}{
		{
			name: "alias and generic columns are quoted with bound values",
			filter: &filter.Filter{LinkOperator: filter.LinkOperatorOr, Items: []filter.FilterItem{
				{Field: "job", Operator: filter.OperatorContains, Value: "aws"},
				{Field: "cluster", Operator: filter.OperatorEquals, Value: "build01"},
			}},
			wantWhere: `WHERE (prow_jobs.name ILIKE $1 OR "cluster" = $2)`,
			wantVars:  []any{"%aws%", "build01"},
		},
		{
			name: "base column not listed as an alias stays filterable",
			filter: &filter.Filter{Items: []filter.FilterItem{
				{Field: "failed", Operator: filter.OperatorEquals, Value: "true"},
			}},
			wantWhere: `WHERE ("failed" = $1)`,
			wantVars:  []any{"true"},
		},
		{
			name: "quote breakout field is a validation error",
			filter: &filter.Filter{Items: []filter.FilterItem{
				{Field: `cluster" = 'x' OR 1=1 --`, Operator: filter.OperatorEquals, Value: "x"},
			}},
			wantValidation:   true,
			wantInvalidField: true,
		},
		{
			name: "negated alias renders constant column with bound pattern",
			filter: &filter.Filter{Items: []filter.FilterItem{
				{Field: "pull_request_author", Operator: filter.OperatorStartsWith, Not: true, Value: "bot_"},
			}},
			wantWhere: `WHERE (NOT(pp.author ILIKE $1))`,
			wantVars:  []any{`bot\_%`},
		},
		{
			name: "unsupported operator on alias column is a validation error",
			filter: &filter.Filter{Items: []filter.FilterItem{
				{Field: "job", Operator: "matches", Value: "x"},
			}},
			wantValidation: true,
		},
		{
			name: "unsupported operator on generic column is a validation error",
			filter: &filter.Filter{Items: []filter.FilterItem{
				{Field: "cluster", Operator: "matches", Value: "x"},
			}},
			wantValidation: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dbc, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 dbname=none sslmode=disable"}),
				&gorm.Config{DryRun: true, DisableAutomaticPing: true})
			require.NoError(t, err)

			opts := &filter.FilterOptions{Filter: tc.filter}
			q, err := applyJobRunFilters(dbc.Table("prow_job_runs"), opts, lookback)
			if tc.wantValidation {
				var validationErr *ValidationError
				require.True(t, errors.As(err, &validationErr), "expected ValidationError, got %v", err)
				if tc.wantInvalidField {
					assert.Contains(t, validationErr.Message, filter.ErrInvalidField.Error())
				}
				return
			}
			require.NoError(t, err)

			var rows []map[string]any
			result := q.Find(&rows)
			require.NoError(t, result.Error)
			assert.Contains(t, result.Statement.SQL.String(), tc.wantWhere)
			assert.Equal(t, tc.wantVars, result.Statement.Vars)
		})
	}
}
