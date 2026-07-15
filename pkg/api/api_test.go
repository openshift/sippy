package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgconn"
	"google.golang.org/api/googleapi"
)

func TestIsBadRequestError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  fmt.Errorf("something went wrong"),
			want: false,
		},
		{
			name: "pg undefined column",
			err:  &pgconn.PgError{Code: "42703"},
			want: true,
		},
		{
			name: "pg other error code",
			err:  &pgconn.PgError{Code: "23505"},
			want: false,
		},
		{
			name: "wrapped pg undefined column",
			err:  fmt.Errorf("query failed: %w", &pgconn.PgError{Code: "42703"}),
			want: true,
		},
		{
			name: "joined pg undefined column",
			err:  errors.Join(fmt.Errorf("first error"), &pgconn.PgError{Code: "42703"}),
			want: true,
		},
		{
			name: "googleapi 400 with invalidQuery reason",
			err: &googleapi.Error{
				Code:   400,
				Errors: []googleapi.ErrorItem{{Reason: bqInvalidQuery, Message: "Unrecognized name: bad_col"}},
			},
			want: true,
		},
		{
			name: "googleapi 400 without invalidQuery reason",
			err: &googleapi.Error{
				Code:   400,
				Errors: []googleapi.ErrorItem{{Reason: "invalid", Message: "some other error"}},
			},
			want: false,
		},
		{
			name: "googleapi 400 with no error items",
			err:  &googleapi.Error{Code: 400},
			want: false,
		},
		{
			name: "googleapi 500",
			err: &googleapi.Error{
				Code:   500,
				Errors: []googleapi.ErrorItem{{Reason: "backendError"}},
			},
			want: false,
		},
		{
			name: "wrapped googleapi 400 invalidQuery",
			err: fmt.Errorf("bq failed: %w", &googleapi.Error{
				Code:   400,
				Errors: []googleapi.ErrorItem{{Reason: bqInvalidQuery}},
			}),
			want: true,
		},
		{
			name: "validation error",
			err:  &ValidationError{Message: "test_id is required"},
			want: true,
		},
		{
			name: "wrapped validation error",
			err:  fmt.Errorf("request failed: %w", &ValidationError{Message: "missing param"}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBadRequestError(tt.err)
			if got != tt.want {
				t.Errorf("IsBadRequestError() = %v, want %v", got, tt.want)
			}
		})
	}
}
