package sippyserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		OverallRisk: apitype.FailureRisk{
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

func TestParseComponentReportRequest(t *testing.T) {

	views := []apitype.ComponentReportView{
		{
			Name: "test view",
			VariantOptions: apitype.ComponentReportRequestVariantOptions{
				GroupBy:  "cloud,arch,network",
				Platform: "",
				Upgrade:  "",
				Arch:     "",
				Network:  "",
				Variant:  "",
			},
			ExcludeOptions: apitype.ComponentReportRequestExcludeOptions{
				ExcludePlatforms: "openstack,ibmcloud,libvirt,ovirt,unknown",
				ExcludeArches:    "arm64,heterogeneous,ppc64le,s390x",
				ExcludeUpgrades:  "",
				ExcludeVariants:  "hypershift,osd,microshift,techpreview,single-node,assisted,compact",
			},
			AdvancedOptions: apitype.ComponentReportRequestAdvancedOptions{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
		},
	}

	tests := []struct {
		name string

		// inputs
		queryParams map[string]string

		// expected outputs
		baseRelease    apitype.ComponentReportRequestReleaseOptions
		sampleRelease  apitype.ComponentReportRequestReleaseOptions
		testIDOption   apitype.ComponentReportRequestTestIdentificationOptions
		variantOption  apitype.ComponentReportRequestVariantOptions
		excludeOption  apitype.ComponentReportRequestExcludeOptions
		advancedOption apitype.ComponentReportRequestAdvancedOptions
		cacheOption    cache.RequestOptions
		errMessage     string
	}{
		{
			name: "normal query params",
			queryParams: map[string]string{
				"baseEndTime":      "2024-02-28T23:59:59Z",
				"baseRelease":      "4.15",
				"baseStartTime":    "2024-02-01T00:00:00Z",
				"confidence":       "95",
				"excludeArches":    "arm64,heterogeneous,ppc64le,s390x",
				"excludeClouds":    "openstack,ibmcloud,libvirt,ovirt,unknown",
				"excludeVariants":  "hypershift,osd,microshift,techpreview,single-node,assisted,compact",
				"groupBy":          "cloud,arch,network",
				"ignoreDisruption": "true",
				"ignoreMissing":    "false",
				"minFail":          "3",
				"pity":             "5",
				"sampleEndTime":    "2024-04-11T23:59:59Z",
				"sampleRelease":    "4.16",
				"sampleStartTime":  "2024-04-04T00:00:05Z",
			},

			baseRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.15",
				Start:   time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				End:     time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.16",
				Start:   time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:     time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: apitype.ComponentReportRequestTestIdentificationOptions{},
			variantOption: apitype.ComponentReportRequestVariantOptions{
				GroupBy:  "cloud,arch,network",
				Platform: "",
				Upgrade:  "",
				Arch:     "",
				Network:  "",
				Variant:  "",
			},
			excludeOption: apitype.ComponentReportRequestExcludeOptions{
				ExcludePlatforms: "openstack,ibmcloud,libvirt,ovirt,unknown",
				ExcludeArches:    "arm64,heterogeneous,ppc64le,s390x",
				ExcludeNetworks:  "",
				ExcludeUpgrades:  "",
				ExcludeVariants:  "hypershift,osd,microshift,techpreview,single-node,assisted,compact",
			},
			advancedOption: apitype.ComponentReportRequestAdvancedOptions{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 4 * time.Hour,
			},
		},
		{
			name: "view",
			queryParams: map[string]string{
				"baseEndTime":     "2024-02-28T23:59:59Z",
				"baseRelease":     "4.15",
				"baseStartTime":   "2024-02-01T00:00:00Z",
				"sampleEndTime":   "2024-04-11T23:59:59Z",
				"sampleRelease":   "4.16",
				"sampleStartTime": "2024-04-04T00:00:05Z",
				"view":            "test view",
			},

			baseRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.15",
				Start:   time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				End:     time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.16",
				Start:   time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:     time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: apitype.ComponentReportRequestTestIdentificationOptions{},
			variantOption: apitype.ComponentReportRequestVariantOptions{
				GroupBy:  "cloud,arch,network",
				Platform: "",
				Upgrade:  "",
				Arch:     "",
				Network:  "",
				Variant:  "",
			},
			excludeOption: apitype.ComponentReportRequestExcludeOptions{
				ExcludePlatforms: "openstack,ibmcloud,libvirt,ovirt,unknown",
				ExcludeArches:    "arm64,heterogeneous,ppc64le,s390x",
				ExcludeNetworks:  "",
				ExcludeUpgrades:  "",
				ExcludeVariants:  "hypershift,osd,microshift,techpreview,single-node,assisted,compact",
			},
			advancedOption: apitype.ComponentReportRequestAdvancedOptions{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 4 * time.Hour,
			},
		},
		{
			name: "view should not be combined with excludeVariants",
			queryParams: map[string]string{
				"baseEndTime":     "2024-02-28T23:59:59Z",
				"baseRelease":     "4.15",
				"baseStartTime":   "2024-02-01T00:00:00Z",
				"sampleEndTime":   "2024-04-11T23:59:59Z",
				"sampleRelease":   "4.16",
				"sampleStartTime": "2024-04-04T00:00:05Z",
				"view":            "test view",
				"excludeVariants": "hypershift,osd,microshift,techpreview,single-node,assisted,compact",
			},

			errMessage: "params cannot be combined with view",
		},
		{
			name: "view should not be combined with groupBy",
			queryParams: map[string]string{
				"baseEndTime":     "2024-02-28T23:59:59Z",
				"baseRelease":     "4.15",
				"baseStartTime":   "2024-02-01T00:00:00Z",
				"sampleEndTime":   "2024-04-11T23:59:59Z",
				"sampleRelease":   "4.16",
				"sampleStartTime": "2024-04-04T00:00:05Z",
				"view":            "test view",
				"groupBy":         "cloud,arch,network",
			},

			errMessage: "params cannot be combined with view",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := url.Values{}
			for k, v := range tc.queryParams {
				params.Set(k, v)
			}

			// path/body are irrelevant at this point in time, we only parse query params in the func being tested
			req, err := http.NewRequest("GET", "https://example.com/path?"+params.Encode(), nil)
			require.NoError(t, err)
			baseRelease, sampleRelease, testIDOption, variantOption, excludeOption, advancedOption, cacheOption, err :=
				parseComponentReportRequest(views, req, 4*time.Hour)

			if tc.errMessage != "" {
				require.Error(t, err)
				require.True(t, strings.Contains(err.Error(), tc.errMessage))
			} else {
				assert.Equal(t, tc.baseRelease, baseRelease)
				assert.Equal(t, tc.sampleRelease, sampleRelease)
				assert.Equal(t, tc.testIDOption, testIDOption)
				assert.Equal(t, tc.variantOption, variantOption)
				assert.Equal(t, tc.excludeOption, excludeOption)
				assert.Equal(t, tc.advancedOption, advancedOption)
				assert.Equal(t, tc.cacheOption, cacheOption)
				assert.Equal(t, tc.baseRelease, baseRelease)
			}
		})
	}

}
