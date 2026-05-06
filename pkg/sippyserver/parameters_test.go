package sippyserver

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPaginationParams(t *testing.T) {
	tests := []struct {
		name      string
		query     url.Values
		wantNil   bool
		wantErr   bool
		wantPage  int
		wantPer   int
	}{
		{
			name:    "no params returns nil",
			query:   url.Values{},
			wantNil: true,
		},
		{
			name:     "valid perPage and page",
			query:    url.Values{"perPage": {"25"}, "page": {"2"}},
			wantPer:  25,
			wantPage: 2,
		},
		{
			name:     "perPage without page defaults to page 0",
			query:    url.Values{"perPage": {"50"}},
			wantPer:  50,
			wantPage: 0,
		},
		{
			name:    "perPage=0 returns error",
			query:   url.Values{"perPage": {"0"}},
			wantErr: true,
		},
		{
			name:    "negative perPage returns error",
			query:   url.Values{"perPage": {"-1"}},
			wantErr: true,
		},
		{
			name:    "negative page returns error",
			query:   url.Values{"perPage": {"25"}, "page": {"-1"}},
			wantErr: true,
		},
		{
			name:    "perPage exceeds max returns error",
			query:   url.Values{"perPage": {"10000"}},
			wantErr: true,
		},
		{
			name:    "non-numeric perPage returns error",
			query:   url.Values{"perPage": {"abc"}},
			wantErr: true,
		},
		{
			name:    "non-numeric page returns error",
			query:   url.Values{"perPage": {"25"}, "page": {"abc"}},
			wantErr: true,
		},
		{
			name:     "max perPage is accepted",
			query:    url.Values{"perPage": {"1000"}},
			wantPer:  1000,
			wantPage: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{URL: &url.URL{RawQuery: tc.query.Encode()}}
			result, err := getPaginationParams(req)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tc.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tc.wantPer, result.PerPage)
			assert.Equal(t, tc.wantPage, result.Page)
		})
	}
}
