package componentreadiness

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
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

	allJobVariants := crtype.JobVariants{Variants: map[string][]string{
		"Architecture": {"amd64", "arm64", "heterogeneous"},
		"FeatureSet":   {"default", "techpreview"},
		"Installer":    {"ipi", "upi"},
		"Network":      {"ovn", "sdn"},
		"Platform":     {"aws", "gcp"},
		"Topology":     {"ha", "single", "microshift", "external"},
		"Upgrade":      {"micro", "minor", "none"},
	}}

	view417main := crtype.View{
		Name: "4.17-main",
		BaseRelease: crtype.RequestRelativeReleaseOptions{
			RequestReleaseOptions: crtype.RequestReleaseOptions{
				Release: "4.16",
			},
			RelativeStart: "ga-30d",
			RelativeEnd:   "ga",
		},
		SampleRelease: crtype.RequestRelativeReleaseOptions{
			RequestReleaseOptions: crtype.RequestReleaseOptions{
				Release: "4.17",
			},
			RelativeStart: "now-7d",
			RelativeEnd:   "now",
		},
		VariantOptions: crtype.RequestVariantOptions{
			ColumnGroupBy:     defaultColumnGroupByVariants,
			DBGroupBy:         defaultDBGroupByVariants,
			IncludeVariants:   includeVariants,
			RequestedVariants: nil,
		},
		AdvancedOptions: crtype.RequestAdvancedOptions{
			MinimumFailure:   3,
			Confidence:       95,
			PityFactor:       5,
			IgnoreMissing:    false,
			IgnoreDisruption: true,
		},
	}
	views := []crtype.View{
		view417main,
	}

	now := time.Now().UTC()
	nowRoundUp := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)

	adjustDur := time.Duration(7) * 24 * time.Hour
	nowMinus7Days := now.Add(-adjustDur)

	tests := []struct {
		name string

		// inputs
		queryParams [][]string

		// expected outputs
		baseRelease    crtype.RequestReleaseOptions
		sampleRelease  crtype.RequestReleaseOptions
		testIDOption   crtype.RequestTestIdentificationOptions
		variantOption  crtype.RequestVariantOptions
		advancedOption crtype.RequestAdvancedOptions
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
			variantOption: crtype.RequestVariantOptions{
				ColumnGroupBy: sets.NewString("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.NewString("Platform", "Architecture", "Network", "Topology", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
				RequestedVariants: map[string]string{},
			},
			baseRelease: crtype.RequestReleaseOptions{
				Release: "4.15",
				Start:   time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				End:     time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: crtype.RequestReleaseOptions{
				Release: "4.16",
				Start:   time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:     time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: crtype.RequestTestIdentificationOptions{},
			advancedOption: crtype.RequestAdvancedOptions{
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
			variantOption: crtype.RequestVariantOptions{
				ColumnGroupBy: sets.NewString("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.NewString("Platform", "Architecture", "Network", "Topology", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
				RequestedVariants: map[string]string{},
			},
			baseRelease: crtype.RequestReleaseOptions{
				Release: "4.15",
				Start:   time.Date(2024, time.January, 29, 0, 0, 0, 0, time.UTC),
				End:     time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: crtype.RequestReleaseOptions{
				Release: "4.16",
				Start:   time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:     time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: crtype.RequestTestIdentificationOptions{},
			advancedOption: crtype.RequestAdvancedOptions{
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
			variantOption: crtype.RequestVariantOptions{
				ColumnGroupBy: sets.NewString("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.NewString("Platform", "Architecture", "Network", "Topology", "Suite", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default", "techpreview"},
					"Installer":    {"ipi", "upi"},
				},
			},
			baseRelease: crtype.RequestReleaseOptions{
				Release: "4.16",
				Start:   time.Date(2024, time.May, 28, 0, 0, 0, 0, time.UTC),
				End:     time.Date(2024, time.June, 27, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: crtype.RequestReleaseOptions{
				Release: "4.17",
				Start:   time.Date(nowMinus7Days.Year(), nowMinus7Days.Month(), nowMinus7Days.Day(), 0, 0, 0, 0, time.UTC),
				End:     nowRoundUp,
			},
			testIDOption: crtype.RequestTestIdentificationOptions{},
			advancedOption: crtype.RequestAdvancedOptions{
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
			options, err := ParseComponentReportRequest(views, req, allJobVariants, time.Duration(0))

			if tc.errMessage != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMessage)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.baseRelease, options.BaseRelease)
				assert.Equal(t, tc.sampleRelease, options.SampleRelease)
				assert.Equal(t, tc.testIDOption, options.TestIDOption)
				assert.Equal(t, tc.variantOption, options.VariantOption)
				assert.Equal(t, tc.advancedOption, options.AdvancedOption)
				assert.Equal(t, tc.cacheOption, options.CacheOption)
				if tc.errMessage != "" {
					assert.Error(t, err)
					assert.True(t, strings.Contains(err.Error(), tc.errMessage))
				}
			}
		})
	}

}
