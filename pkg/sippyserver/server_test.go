package sippyserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestSippyNGRedirectRemainsSameOrigin(t *testing.T) {
	tests := []struct {
		name      string
		rawQuery  string
		wantQuery string
	}{
		{name: "no query"},
		{
			name:      "ordinary query",
			rawQuery:  "release=4.21&arch=amd64",
			wantQuery: "arch=amd64&release=4.21",
		},
		{
			name:      "duplicate values",
			rawQuery:  "filter=first&filter=second&arch=amd64",
			wantQuery: "arch=amd64&filter=first&filter=second",
		},
		{
			name:      "malformed pairs are omitted",
			rawQuery:  "valid=kept&broken=%zz&semi=not;valid&also=kept",
			wantQuery: "also=kept&valid=kept",
		},
		{
			name:      "absolute URL in value",
			rawQuery:  "next=https://attacker.example/path",
			wantQuery: "next=https%3A%2F%2Fattacker.example%2Fpath",
		},
		{
			name:      "network path in value",
			rawQuery:  "next=//attacker.example/path",
			wantQuery: "next=%2F%2Fattacker.example%2Fpath",
		},
		{
			name:      "encoded header injection in value",
			rawQuery:  "next=%0d%0aLocation:%20https://attacker.example",
			wantQuery: "next=%0D%0ALocation%3A+https%3A%2F%2Fattacker.example",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://sippy.test/sippy-ng", nil)
			request.URL.RawQuery = tc.rawQuery
			response := httptest.NewRecorder()
			router := mux.NewRouter()
			router.StrictSlash(true)
			(&Server{}).registerSippyNGRoutes(router)
			router.ServeHTTP(response, request)

			if response.Code != http.StatusMovedPermanently {
				t.Fatalf("redirect status = %d, want %d", response.Code, http.StatusMovedPermanently)
			}
			if got := response.Header().Get("Content-Type"); got != "" {
				t.Fatalf("redirect Content-Type = %q, want empty", got)
			}
			if response.Body.Len() != 0 {
				t.Fatalf("redirect body = %q, want empty", response.Body.String())
			}

			location := response.Header().Get("Location")
			if strings.ContainsAny(location, "\r\n") {
				t.Fatalf("redirect Location contains a header delimiter: %q", location)
			}
			parsed, err := url.Parse(location)
			if err != nil {
				t.Fatalf("parse redirect target: %v", err)
			}
			if parsed.IsAbs() || parsed.Host != "" || parsed.Scheme != "" {
				t.Fatalf("redirect target must be same-origin: %q", location)
			}
			if parsed.Path != "/sippy-ng/" {
				t.Fatalf("redirect path = %q, want /sippy-ng/", parsed.Path)
			}
			if parsed.RawQuery != tc.wantQuery {
				t.Fatalf("redirect query = %q, want canonical query %q", parsed.RawQuery, tc.wantQuery)
			}
			if parsed.Query().Encode() != tc.wantQuery {
				t.Fatalf("redirect query values = %q, want %q", parsed.Query().Encode(), tc.wantQuery)
			}
		})
	}
}

func TestValidateCRDataSource(t *testing.T) {
	for _, dataSource := range []string{"", reqopts.DataSourceBigQuery, reqopts.DataSourcePostgres} {
		t.Run(dataSource, func(t *testing.T) {
			if err := validateCRDataSource(dataSource); err != nil {
				t.Fatalf("validateCRDataSource(%q) returned error: %v", dataSource, err)
			}
		})
	}

	err := validateCRDataSource("bigquey")
	if err == nil {
		t.Fatal("validateCRDataSource(bigquey) returned nil")
	}
	if !strings.Contains(err.Error(), "invalid dataSource") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "must be empty, \"bigquery\", or \"postgres\"") {
		t.Fatalf("error does not describe all valid values: %v", err)
	}
}

func TestValidateProwJobRun(t *testing.T) {

	tests := []struct {
		name                 string
		prowJobRun           *models.ProwJobRun
		expectedValidation   bool
		expectedDetailReason string
	}{
		{
			// no prowJobRun specified
			// simulates what we are seeing from the origin riskanalysis command
			// when missing junit artifacts
			name:                 "Test Nil ProwJobRun",
			expectedValidation:   false,
			expectedDetailReason: "empty ProwJobRun",
		},
		{
			prowJobRun:           &models.ProwJobRun{},
			name:                 "Test Empty ProwJobRun",
			expectedValidation:   false,
			expectedDetailReason: "missing ProwJob Name",
		},
		{
			prowJobRun:           &models.ProwJobRun{ProwJob: models.ProwJob{}},
			name:                 "Test Empty ProwJob",
			expectedValidation:   false,
			expectedDetailReason: "missing ProwJob Name",
		},
		{
			prowJobRun:         &models.ProwJobRun{ProwJob: models.ProwJob{Name: "test"}},
			name:               "Test Valid ProwJob",
			expectedValidation: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// no files found so we marshall the null prowJobRun
			inputBytes, err := json.Marshal(tc.prowJobRun)

			// we don't encounter an error
			if err != nil {
				t.Fatalf("Error marshalling prowjob for %s", tc.name)
			}

			// string 'null' not nil bytes if we have a nil pointer
			if inputBytes == nil {
				t.Fatalf("Nil Bytes for %s", tc.name)
			}

			jobRun := &models.ProwJobRun{}

			// we decode the string 'null' but we don't get an error...
			err = json.NewDecoder(strings.NewReader(string(inputBytes))).Decode(&jobRun)

			if err != nil {
				t.Fatalf("Error decoding prowjob for %s", tc.name)
			}

			isValid, detailReason := isValidProwJobRun(jobRun)

			if isValid != tc.expectedValidation {
				t.Fatalf("Validation %t did not match expected Expected %t for %s", isValid, tc.expectedValidation, tc.name)
			}

			if detailReason != tc.expectedDetailReason {
				t.Fatalf("DetailReason %s did not match Expected %s for %s", detailReason, tc.expectedDetailReason, tc.name)
			}

		})
	}

}

func TestEncodeDefaultHighRisk(t *testing.T) {
	result := apitype.ProwJobRunRiskAnalysis{
		OverallRisk: apitype.JobFailureRisk{
			Level:   apitype.FailureRiskLevelHigh,
			Reasons: []string{"Invalid ProwJob provided for analysis"},
		},
	}

	encodedRiskResult, err := json.Marshal(result)

	if err != nil {
		t.Fatal("Error while encoding risk analysis")
	}

	riskResultJSON := string(encodedRiskResult)

	if riskResultJSON == "" {
		t.Fatal("Invalid risk analysis json")
	}

	analysis := &apitype.ProwJobRunRiskAnalysis{}

	err = json.NewDecoder(strings.NewReader(string(encodedRiskResult))).Decode(&analysis)

	if err != nil {
		t.Fatal("Error while decoding risk analysis")
	}

	if analysis == nil {
		t.Fatal("Invalid risk analysis after decoding")
	}

	if analysis.OverallRisk.Level.Level != apitype.FailureRiskLevelHigh.Level {
		t.Fatal("Invalid overall risk analysis after decoding")
	}
}
