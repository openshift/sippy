package componentreadiness

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	includeVariants = map[string][]string{
		"Architecture": []string{"amd64"},
		"FeatureSet":   []string{"default", "techpreview"},
		"Installer":    []string{"ipi", "upi"},
	}
)

func TestParseComponentReportRequest(t *testing.T) {

	allJobVariants := apitype.JobVariants{Variants: map[string][]string{
		"Architecture": {"amd64", "arm64", "heterogeneous"},
		"FeatureSet":   {"default", "techpreview"},
		"Installer":    {"ipi", "upi"},
		"Network":      {"ovn", "sdn"},
		"Platform":     {"aws", "gcp"},
		"Topology":     {"ha", "single", "microshift", "external"},
		"Upgrade":      {"micro", "minor", "none"},
	}}

	view417main := apitype.ComponentReportView{
		Name: "4.17-main",
		BaseRelease: apitype.ComponentReportRequestRelativeReleaseOptions{
			Release:       "4.16",
			RelativeStart: "ga-30d",
			RelativeEnd:   "ga",
		},
		SampleRelease: apitype.ComponentReportRequestRelativeReleaseOptions{
			Release:       "4.17",
			RelativeStart: "now-7d",
			RelativeEnd:   "now",
		},
		VariantOptions: apitype.ComponentReportRequestVariantOptions{
			ColumnGroupBy:     defaultColumnGroupByVariants,
			DBGroupBy:         defaultDBGroupByVariants,
			IncludeVariants:   includeVariants,
			RequestedVariants: nil,
		},
		AdvancedOptions: apitype.ComponentReportRequestAdvancedOptions{
			MinimumFailure:   3,
			Confidence:       95,
			PityFactor:       5,
			IgnoreMissing:    false,
			IgnoreDisruption: true,
		},
	}
	views := []apitype.ComponentReportView{
		view417main,
	}

	now := time.Now().UTC()
	//nowRoundDown := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	nowRoundUp := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)

	tests := []struct {
		name string

		// inputs
		queryParams [][]string

		// expected outputs
		baseRelease    apitype.ComponentReportRequestReleaseOptions
		sampleRelease  apitype.ComponentReportRequestReleaseOptions
		testIDOption   apitype.ComponentReportRequestTestIdentificationOptions
		variantOption  apitype.ComponentReportRequestVariantOptions
		advancedOption apitype.ComponentReportRequestAdvancedOptions
		cacheOption    cache.RequestOptions
		errMessage     string
	}{
		{
			name: "normal query params",
			queryParams: [][]string{
				{"baseEndTime", "2024-02-28T23:59:59Z"},
				{"baseRelease", "4.15"},
				{"baseStartTime", "2024-02-01T00:00:00Z"},
				{"confidence", "95"},
				{"groupBy", "cloud,arch,network"},
				{"columnGroupBy", "Platform,Architecture,Network"},
				{"dbGroupBy", "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Installer"},
				{"ignoreDisruption", "true"},
				{"ignoreMissing", "false"},
				{"minFail", "3"},
				{"pity", "5"},
				{"sampleEndTime", "2024-04-11T23:59:59Z"},
				{"sampleRelease", "4.16"},
				{"sampleStartTime", "2024-04-04T00:00:05Z"},
				{"includeVariant", "Architecture:amd64"},
				{"includeVariant", "FeatureSet:default"},
				{"includeVariant", "Installer:ipi"},
				{"includeVariant", "Installer:upi"},
			},
			variantOption: apitype.ComponentReportRequestVariantOptions{
				ColumnGroupBy: sets.NewString("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.NewString("Platform", "Architecture", "Network", "Topology", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
				RequestedVariants: map[string]string{},
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
			advancedOption: apitype.ComponentReportRequestAdvancedOptions{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh: false,
			},
		},
		{
			name: "relative time query params",
			queryParams: [][]string{
				{"baseEndTime", "ga"},
				{"baseRelease", "4.15"},
				{"baseStartTime", "ga-30d"},
				{"confidence", "95"},
				{"groupBy", "cloud,arch,network"},
				{"columnGroupBy", "Platform,Architecture,Network"},
				{"dbGroupBy", "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Installer"},
				{"ignoreDisruption", "true"},
				{"ignoreMissing", "false"},
				{"minFail", "3"},
				{"pity", "5"},
				{"sampleEndTime", "2024-04-11T23:59:59Z"},
				{"sampleRelease", "4.16"},
				{"sampleStartTime", "2024-04-04T00:00:05Z"},
				{"includeVariant", "Architecture:amd64"},
				{"includeVariant", "FeatureSet:default"},
				{"includeVariant", "Installer:ipi"},
				{"includeVariant", "Installer:upi"},
			},
			variantOption: apitype.ComponentReportRequestVariantOptions{
				ColumnGroupBy: sets.NewString("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.NewString("Platform", "Architecture", "Network", "Topology", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
				RequestedVariants: map[string]string{},
			},
			baseRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.15",
				Start:   time.Date(2024, time.January, 29, 0, 0, 0, 0, time.UTC),
				End:     time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.16",
				Start:   time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:     time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: apitype.ComponentReportRequestTestIdentificationOptions{},
			advancedOption: apitype.ComponentReportRequestAdvancedOptions{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh: false,
			},
		},
		{
			name: "basic view",
			queryParams: [][]string{
				{"view", "4.17-main"},
			},
			variantOption: apitype.ComponentReportRequestVariantOptions{
				ColumnGroupBy: sets.NewString("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.NewString("Platform", "Architecture", "Network", "Topology", "Suite", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default", "techpreview"},
					"Installer":    {"ipi", "upi"},
				},
			},
			baseRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.16",
				Start:   time.Date(2024, time.May, 28, 0, 0, 0, 0, time.UTC),
				End:     time.Date(2024, time.June, 27, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: apitype.ComponentReportRequestReleaseOptions{
				Release: "4.17",
				Start:   time.Date(2024, time.July, 26, 0, 0, 0, 0, time.UTC),
				End:     nowRoundUp,
			},
			testIDOption: apitype.ComponentReportRequestTestIdentificationOptions{},
			advancedOption: apitype.ComponentReportRequestAdvancedOptions{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh: false,
			},
		},
		{
			name: "non-existent view",
			queryParams: [][]string{
				{"view", "4.13-main"}, // doesn't exist
			},
			errMessage: "unknown view",
		},
		{
			name: "cannot combine view and includeVariant",
			queryParams: [][]string{
				{"view", "4.17-main"}, // doesn't exist
				{"includeVariant", "Topology:single"},
			},
			errMessage: "params cannot be combined with view",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := url.Values{}
			for _, tuple := range tc.queryParams {
				params.Add(tuple[0], tuple[1])
			}

			// path/body are irrelevant at this point in time, we only parse query params in the func being tested
			req, err := http.NewRequest("GET", "https://example.com/path?"+params.Encode(), nil)
			require.NoError(t, err)
			baseRelease, sampleRelease, testIDOption, variantOption, advancedOption, cacheOption, err :=
				ParseComponentReportRequest(views, req, allJobVariants, time.Duration(0))

			if tc.errMessage != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMessage)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.baseRelease, baseRelease)
				assert.Equal(t, tc.sampleRelease, sampleRelease)
				assert.Equal(t, tc.testIDOption, testIDOption)
				assert.Equal(t, tc.variantOption, variantOption)
				assert.Equal(t, tc.advancedOption, advancedOption)
				assert.Equal(t, tc.cacheOption, cacheOption)
				if tc.errMessage != "" {
					assert.Error(t, err)
					assert.True(t, strings.Contains(err.Error(), tc.errMessage))
				}
				assert.Equal(t, tc.baseRelease, baseRelease)
			}
		})
	}

}
