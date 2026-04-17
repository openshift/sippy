package param

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SQL injection characters stripped",
			input:    "'; DROP TABLE--",
			expected: " DROP TABLE--",
		},
		{
			name:     "spaces are allowed",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "all allowed characters pass through",
			input:    "test:value-1_2",
			expected: "test:value-1_2",
		},
		{
			name:     "unicode stripped entirely",
			input:    "名前",
			expected: "",
		},
		{
			name:     "empty stays empty",
			input:    "",
			expected: "",
		},
		{
			name:     "mixed allowed and disallowed",
			input:    "abc!@#123",
			expected: "abc123",
		},
		{
			name:     "only alphanumeric",
			input:    "SimpleTest123",
			expected: "SimpleTest123",
		},
		{
			name:     "tabs and newlines stripped",
			input:    "line1\tline2\nline3",
			expected: "line1line2line3",
		},
		{
			name:     "parentheses and brackets stripped",
			input:    "func(arg)[0]",
			expected: "funcarg0",
		},
		{
			name:     "percent and equals stripped",
			input:    "key=value%20encoded",
			expected: "keyvalue20encoded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Cleanse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSafeRead(t *testing.T) {
	tests := []struct {
		name      string
		paramName string
		value     string
		expected  string
	}{
		// release param: ^[\w.-]+$
		{
			name:      "release valid dotted version",
			paramName: "release",
			value:     "4.16",
			expected:  "4.16",
		},
		{
			name:      "release rejects SQL injection",
			paramName: "release",
			value:     "4.16; DROP",
			expected:  "",
		},
		{
			name:      "release valid word",
			paramName: "release",
			value:     "Presubmit",
			expected:  "Presubmit",
		},

		// baseRelease param: ^\d+\.\d+$
		{
			name:      "baseRelease valid",
			paramName: "baseRelease",
			value:     "4.16",
			expected:  "4.16",
		},
		{
			name:      "baseRelease rejects triple dot",
			paramName: "baseRelease",
			value:     "4.16.1",
			expected:  "",
		},
		{
			name:      "baseRelease rejects v prefix",
			paramName: "baseRelease",
			value:     "v4.16",
			expected:  "",
		},

		// prow_job_run_id param: ^\d+$
		{
			name:      "prow_job_run_id valid digits",
			paramName: "prow_job_run_id",
			value:     "12345",
			expected:  "12345",
		},
		{
			name:      "prow_job_run_id rejects alphanumeric",
			paramName: "prow_job_run_id",
			value:     "123abc",
			expected:  "",
		},

		// test_id param: ^[\w:. -]+$
		{
			name:      "test_id valid with colon and hex",
			paramName: "test_id",
			value:     "openshift-tests-upgrade:af8a62c5",
			expected:  "openshift-tests-upgrade:af8a62c5",
		},
		{
			name:      "test_id valid with space",
			paramName: "test_id",
			value:     "cluster install:0cb1bb27",
			expected:  "cluster install:0cb1bb27",
		},

		// view param: ^[-.\w]+$
		{
			name:      "view valid with dash",
			paramName: "view",
			value:     "4.16-main",
			expected:  "4.16-main",
		},
		{
			name:      "view rejects space",
			paramName: "view",
			value:     "view name",
			expected:  "",
		},

		// include_success param: ^(true|false)$
		{
			name:      "include_success true",
			paramName: "include_success",
			value:     "true",
			expected:  "true",
		},
		{
			name:      "include_success false",
			paramName: "include_success",
			value:     "false",
			expected:  "false",
		},
		{
			name:      "include_success rejects uppercase True",
			paramName: "include_success",
			value:     "True",
			expected:  "",
		},

		// empty value always returns ""
		{
			name:      "empty value returns empty string",
			paramName: "release",
			value:     "",
			expected:  "",
		},

		// prow_job_run_ids param: ^\d+(,\d+)*$
		{
			name:      "prow_job_run_ids single value",
			paramName: "prow_job_run_ids",
			value:     "12345",
			expected:  "12345",
		},
		{
			name:      "prow_job_run_ids comma separated",
			paramName: "prow_job_run_ids",
			value:     "123,456,789",
			expected:  "123,456,789",
		},
		{
			name:      "prow_job_run_ids rejects trailing comma",
			paramName: "prow_job_run_ids",
			value:     "123,",
			expected:  "",
		},

		// start_date param: ^\d{4}-\d{2}-\d{2}$
		{
			name:      "start_date valid format",
			paramName: "start_date",
			value:     "2024-01-15",
			expected:  "2024-01-15",
		},
		{
			name:      "start_date rejects wrong format",
			paramName: "start_date",
			value:     "01-15-2024",
			expected:  "",
		},

		// test param: ^.+$ (anything non-empty)
		{
			name:      "test allows anything",
			paramName: "test",
			value:     "[sig-cli] oc explain should work",
			expected:  "[sig-cli] oc explain should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := "http://example.com/api"
			if tt.value != "" {
				u += "?" + tt.paramName + "=" + url.QueryEscape(tt.value)
			}
			req := httptest.NewRequest("GET", u, nil)
			result := SafeRead(req, tt.paramName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReadUint(t *testing.T) {
	tests := []struct {
		name        string
		paramValue  string
		limit       int
		expectedVal int
		expectErr   bool
	}{
		{
			name:        "missing param returns zero and nil",
			paramValue:  "",
			limit:       100,
			expectedVal: 0,
			expectErr:   false,
		},
		{
			name:        "valid value within limit",
			paramValue:  "42",
			limit:       100,
			expectedVal: 42,
			expectErr:   false,
		},
		{
			name:        "value exceeds limit",
			paramValue:  "42",
			limit:       10,
			expectedVal: 0,
			expectErr:   true,
		},
		{
			name:        "limit zero means no limit",
			paramValue:  "42",
			limit:       0,
			expectedVal: 42,
			expectErr:   false,
		},
		{
			name:        "negative value rejected by regex",
			paramValue:  "-5",
			limit:       100,
			expectedVal: 0,
			expectErr:   true,
		},
		{
			name:        "non-numeric rejected",
			paramValue:  "abc",
			limit:       100,
			expectedVal: 0,
			expectErr:   true,
		},
		{
			name:        "value exactly at limit",
			paramValue:  "100",
			limit:       100,
			expectedVal: 100,
			expectErr:   false,
		},
		{
			name:        "value one over limit",
			paramValue:  "101",
			limit:       100,
			expectedVal: 0,
			expectErr:   true,
		},
		{
			name:        "zero value",
			paramValue:  "0",
			limit:       100,
			expectedVal: 0,
			expectErr:   false,
		},
		{
			name:        "decimal rejected by regex",
			paramValue:  "3.14",
			limit:       100,
			expectedVal: 0,
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := "http://example.com/api"
			if tt.paramValue != "" {
				u += "?count=" + url.QueryEscape(tt.paramValue)
			}
			req := httptest.NewRequest("GET", u, nil)
			val, err := ReadUint(req, "count", tt.limit)
			if tt.expectErr {
				require.Error(t, err)
				assert.Equal(t, 0, val)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedVal, val)
			}
		})
	}
}

func TestReadBool(t *testing.T) {
	tests := []struct {
		name         string
		paramValue   string
		defaultValue bool
		expectedVal  bool
		expectErr    bool
	}{
		{
			name:         "empty with default true",
			paramValue:   "",
			defaultValue: true,
			expectedVal:  true,
			expectErr:    false,
		},
		{
			name:         "empty with default false",
			paramValue:   "",
			defaultValue: false,
			expectedVal:  false,
			expectErr:    false,
		},
		{
			name:         "true value",
			paramValue:   "true",
			defaultValue: false,
			expectedVal:  true,
			expectErr:    false,
		},
		{
			name:         "false value",
			paramValue:   "false",
			defaultValue: true,
			expectedVal:  false,
			expectErr:    false,
		},
		{
			name:         "uppercase TRUE rejected",
			paramValue:   "TRUE",
			defaultValue: false,
			expectedVal:  false,
			expectErr:    true,
		},
		{
			name:         "numeric 1 rejected",
			paramValue:   "1",
			defaultValue: false,
			expectedVal:  false,
			expectErr:    true,
		},
		{
			name:         "yes rejected",
			paramValue:   "yes",
			defaultValue: false,
			expectedVal:  false,
			expectErr:    true,
		},
		{
			name:         "mixed case True rejected",
			paramValue:   "True",
			defaultValue: false,
			expectedVal:  false,
			expectErr:    true,
		},
		{
			name:         "numeric 0 rejected",
			paramValue:   "0",
			defaultValue: true,
			expectedVal:  false,
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := "http://example.com/api"
			if tt.paramValue != "" {
				u += "?flag=" + url.QueryEscape(tt.paramValue)
			}
			req := httptest.NewRequest("GET", u, nil)
			val, err := ReadBool(req, "flag", tt.defaultValue)
			if tt.expectErr {
				require.Error(t, err)
				assert.Equal(t, false, val)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedVal, val)
			}
		})
	}
}
