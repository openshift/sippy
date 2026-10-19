package sippyserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestMCPRouteIsNotExposed(t *testing.T) {
	server := &Server{static: fstest.MapFS{}}
	handler := server.handler()

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		for _, path := range []string{"/mcp/v1", "/mcp/v1/", "/mcp/v1/tools"} {
			t.Run(method+" "+path, func(t *testing.T) {
				request := httptest.NewRequest(method, path, nil)
				response := httptest.NewRecorder()

				handler.ServeHTTP(response, request)

				if response.Code != http.StatusNotFound {
					t.Fatalf("expected %s %s to return %d, got %d", method, path, http.StatusNotFound, response.Code)
				}
			})
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected API index to return %d, got %d", http.StatusOK, response.Code)
	}

	var endpoints []struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(response.Body).Decode(&endpoints); err != nil {
		t.Fatalf("decode API index: %v", err)
	}
	for _, endpoint := range endpoints {
		if strings.HasPrefix(endpoint.Path, "/mcp/") {
			t.Fatalf("API index still advertises MCP endpoint %q", endpoint.Path)
		}
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
