package componentreadiness

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	includeVariants = map[string][]string{
		"Architecture": {"amd64"},
		"FeatureSet":   {"default", "techpreview"},
		"Installer":    {"ipi", "upi"},
	}
)

func TestParseComponentReportRequest(t *testing.T) {

	releases := []v1.Release{
		{Release: "4.16", Status: "", GADate: util.DatePtr(2024, 6, 27, 0, 0, 0, 0, time.UTC)},
		{Release: "4.15", Status: "", GADate: util.DatePtr(2024, 2, 28, 0, 0, 0, 0, time.UTC)},
	}

	allJobVariants := crtest.JobVariants{Variants: map[string][]string{
		"Architecture": {"amd64", "arm64", "s390x", "ppc64le", "heterogeneous"},
		"FeatureSet":   {"default", "techpreview"},
		"Installer":    {"ipi", "upi"},
		"Network":      {"ovn", "sdn"},
		"Platform":     {"aws", "gcp"},
		"Topology":     {"ha", "single", "microshift", "external"},
		"Upgrade":      {"micro", "minor", "none"},
	}}

	view417main := crview.View{
		Name: "4.17-main",
		BaseRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.16",
			},
			RelativeStart: "ga-30d",
			RelativeEnd:   "ga",
		},
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.17",
			},
			RelativeStart: "now-7d",
			RelativeEnd:   "now",
		},
		VariantOptions: reqopts.Variants{
			ColumnGroupBy:   defaultColumnGroupByVariants,
			DBGroupBy:       defaultDBGroupByVariants,
			IncludeVariants: includeVariants,
		},
		AdvancedOptions: reqopts.Advanced{
			MinimumFailure:   3,
			Confidence:       95,
			PityFactor:       5,
			IgnoreMissing:    false,
			IgnoreDisruption: true,
		},
	}
	// would like to test with a view that does define cross-compare variants
	view417cross := view417main
	view417cross.Name = "4.17-cross"
	view417cross.VariantOptions = reqopts.Variants{
		VariantCrossCompare: []string{"Topology"},
		IncludeVariants: map[string][]string{
			"Architecture": {"amd64"},
			"Installer":    {"ipi", "upi"},
			"Topology":     {"ha"},
		},
		CompareVariants: map[string][]string{
			"Architecture": {"amd64"},
			"Installer":    {"ipi", "upi"},
			"Topology":     {"single"},
		},
		// also remove Topology from columnGroupBy and dbGroupBy
		ColumnGroupBy: sets.New("Platform", "Architecture", "Network"),
		DBGroupBy:     sets.New("Platform", "Architecture", "Network", "Suite", "FeatureSet", "Upgrade", "Installer"),
	}

	views := []crview.View{
		view417main,
		view417cross,
	}

	now := time.Now().UTC()
	nowTruncatedAligned := util.TruncateAligned(now, 12*time.Hour, 4*time.Hour)

	adjustDur := time.Duration(7) * 24 * time.Hour
	nowMinus7Days := now.Add(-adjustDur)

	tests := []struct {
		name string

		// inputs
		queryParams [][]string

		// expected outputs
		baseRelease     reqopts.Release
		sampleRelease   reqopts.Release
		testIDOption    reqopts.TestIdentification
		variantOption   reqopts.Variants
		advancedOption  reqopts.Advanced
		cacheOption     cache.RequestOptions
		includeAllTests bool
		errMessage      string
	}{
		{
			name: "normal query params",
			queryParams: [][]string{
				{"baseEndTime", "2024-02-28T23:59:59Z"},
				{"baseRelease", "4.15"},
				{"baseStartTime", "2024-02-01T00:00:00Z"},
				{"confidence", "95"},
				{"columnGroupBy", "Platform,Architecture,Network"},
				{"dbGroupBy", "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Installer"},
				{"ignoreDisruption", "true"},
				{"ignoreMissing", "false"},
				{"includeAllTests", "true"},
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
			includeAllTests: true,
			variantOption: reqopts.Variants{
				ColumnGroupBy: sets.New("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.New("Platform", "Architecture", "Network", "Topology", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
			},
			baseRelease: reqopts.Release{
				Name:  "4.15",
				Start: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: reqopts.Release{
				Name:  "4.16",
				Start: time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:   time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: reqopts.TestIdentification{
				RequestedVariants: map[string]string{},
			},
			advancedOption: reqopts.Advanced{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 12 * time.Hour,
				CRTimeRoundingOffset: 4 * time.Hour,
				StableAge:            cache.StandardStableAgeCR,
				StableExpiry:         cache.StandardStableExpiryCR,
			},
		},
		{
			name: "relative time query params",
			queryParams: [][]string{
				{"baseEndTime", "ga"},
				{"baseRelease", "4.15"},
				{"baseStartTime", "ga-30d"},
				{"confidence", "95"},
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
			variantOption: reqopts.Variants{
				ColumnGroupBy: sets.New("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.New("Platform", "Architecture", "Network", "Topology", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
			},
			baseRelease: reqopts.Release{
				Name:  "4.15",
				Start: time.Date(2024, time.January, 29, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: reqopts.Release{
				Name:  "4.16",
				Start: time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:   time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: reqopts.TestIdentification{
				RequestedVariants: map[string]string{},
			},
			advancedOption: reqopts.Advanced{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 12 * time.Hour,
				CRTimeRoundingOffset: 4 * time.Hour,
				StableAge:            cache.StandardStableAgeCR,
				StableExpiry:         cache.StandardStableExpiryCR,
			},
		},
		{
			name: "basic view",
			queryParams: [][]string{
				{"view", "4.17-main"},
			},
			variantOption: reqopts.Variants{
				ColumnGroupBy: sets.New("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.New("Platform", "Architecture", "Network", "Topology", "Suite", "FeatureSet", "Upgrade", "Installer", "LayeredProduct"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"FeatureSet":   {"default", "techpreview"},
					"Installer":    {"ipi", "upi"},
				},
				CompareVariants: nil, // the view is likely not to specify compare variants at all
			},
			baseRelease: reqopts.Release{
				Name:  "4.16",
				Start: time.Date(2024, time.May, 28, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, time.June, 27, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: reqopts.Release{
				Name:  "4.17",
				Start: time.Date(nowMinus7Days.Year(), nowMinus7Days.Month(), nowMinus7Days.Day(), 0, 0, 0, 0, time.UTC),
				End:   nowTruncatedAligned,
			},
			testIDOption: reqopts.TestIdentification{
				RequestedVariants: map[string]string{},
			},
			advancedOption: reqopts.Advanced{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 12 * time.Hour,
				CRTimeRoundingOffset: 4 * time.Hour,
				StableAge:            cache.StandardStableAgeCR,
				StableExpiry:         cache.StandardStableExpiryCR,
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
			name: "view with includeVariant override replaces view defaults",
			queryParams: [][]string{
				{"view", "4.17-main"},
				{"includeVariant", "Platform:gcp"},
				{"includeVariant", "Topology:single"},
			},
			variantOption: reqopts.Variants{
				ColumnGroupBy: sets.New("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.New("Platform", "Architecture", "Network", "Topology", "Suite", "FeatureSet", "Upgrade", "Installer", "LayeredProduct"),
				// URL params completely replace view's includeVariants
				IncludeVariants: map[string][]string{
					"Platform": {"gcp"},
					"Topology": {"single"},
				},
				CompareVariants: nil,
			},
			baseRelease: reqopts.Release{
				Name:  "4.16",
				Start: time.Date(2024, time.May, 28, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, time.June, 27, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: reqopts.Release{
				Name:  "4.17",
				Start: time.Date(nowMinus7Days.Year(), nowMinus7Days.Month(), nowMinus7Days.Day(), 0, 0, 0, 0, time.UTC),
				End:   nowTruncatedAligned,
			},
			testIDOption: reqopts.TestIdentification{
				RequestedVariants: map[string]string{},
			},
			advancedOption: reqopts.Advanced{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 12 * time.Hour,
				CRTimeRoundingOffset: 4 * time.Hour,
				StableAge:            cache.StandardStableAgeCR,
				StableExpiry:         cache.StandardStableExpiryCR,
			},
		},
		{
			name: "normal query params but with variant cross-compare",
			queryParams: [][]string{
				{"baseEndTime", "2024-02-28T23:59:59Z"},
				{"baseRelease", "4.15"},
				{"baseStartTime", "2024-02-01T00:00:00Z"},
				{"columnGroupBy", "Platform,Network"},
				{"dbGroupBy", "Platform,Network,FeatureSet,Upgrade,Installer"},
				{"sampleEndTime", "2024-04-11T23:59:59Z"},
				{"sampleRelease", "4.16"},
				{"sampleStartTime", "2024-04-04T00:00:05Z"},
				{"includeVariant", "Architecture:amd64"},
				{"includeVariant", "Architecture:arm64"},
				{"includeVariant", "Topology:ha"},
				{"includeVariant", "FeatureSet:default"},
				{"includeVariant", "Installer:ipi"},
				{"includeVariant", "Installer:upi"},
				{"variantCrossCompare", "Architecture"},
				{"variantCrossCompare", "Topology"},
				{"compareVariant", "Architecture:s390x"},
				{"compareVariant", "Architecture:ppc64le"},
				{"compareVariant", "Topology:single"},
			},
			variantOption: reqopts.Variants{
				ColumnGroupBy: sets.New("Platform", "Network"),
				DBGroupBy:     sets.New("Platform", "Network", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64", "arm64"},
					"Topology":     {"ha"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
				CompareVariants: map[string][]string{
					"Architecture": {"s390x", "ppc64le"},
					"Topology":     {"single"},
					"FeatureSet":   {"default"},
					"Installer":    {"ipi", "upi"},
				},
				VariantCrossCompare: []string{"Architecture", "Topology"},
			},
			baseRelease: reqopts.Release{
				Name:  "4.15",
				Start: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, time.February, 28, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: reqopts.Release{
				Name:  "4.16",
				Start: time.Date(2024, time.April, 4, 0, 0, 5, 0, time.UTC),
				End:   time.Date(2024, time.April, 11, 23, 59, 59, 0, time.UTC),
			},
			testIDOption: reqopts.TestIdentification{
				RequestedVariants: map[string]string{},
			},
			advancedOption: reqopts.Advanced{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 12 * time.Hour,
				CRTimeRoundingOffset: 4 * time.Hour,
				StableAge:            cache.StandardStableAgeCR,
				StableExpiry:         cache.StandardStableExpiryCR,
			},
		},
		{
			name: "cross-compare view",
			queryParams: [][]string{
				{"view", "4.17-cross"},
			},
			variantOption: reqopts.Variants{
				ColumnGroupBy: sets.New("Platform", "Architecture", "Network"),
				DBGroupBy:     sets.New("Platform", "Architecture", "Network", "Suite", "FeatureSet", "Upgrade", "Installer"),
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"Installer":    {"ipi", "upi"},
					"Topology":     {"ha"},
				},
				VariantCrossCompare: []string{"Topology"},
				CompareVariants: map[string][]string{
					"Architecture": {"amd64"},
					"Installer":    {"ipi", "upi"},
					"Topology":     {"single"},
				},
			},
			baseRelease: reqopts.Release{
				Name:  "4.16",
				Start: time.Date(2024, time.May, 28, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, time.June, 27, 23, 59, 59, 0, time.UTC),
			},
			sampleRelease: reqopts.Release{
				Name:  "4.17",
				Start: time.Date(nowMinus7Days.Year(), nowMinus7Days.Month(), nowMinus7Days.Day(), 0, 0, 0, 0, time.UTC),
				End:   nowTruncatedAligned,
			},
			testIDOption: reqopts.TestIdentification{
				RequestedVariants: map[string]string{},
			},
			advancedOption: reqopts.Advanced{
				MinimumFailure:   3,
				Confidence:       95,
				PityFactor:       5,
				IgnoreMissing:    false,
				IgnoreDisruption: true,
			},
			cacheOption: cache.RequestOptions{
				ForceRefresh:         false,
				CRTimeRoundingFactor: 12 * time.Hour,
				CRTimeRoundingOffset: 4 * time.Hour,
				StableAge:            cache.StandardStableAgeCR,
				StableExpiry:         cache.StandardStableExpiryCR,
			},
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
			options, _, err := utils.ParseComponentReportRequest(views, releases, req, allJobVariants, 12*time.Hour, 4*time.Hour)

			if tc.errMessage != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMessage)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.baseRelease, options.BaseRelease)
				assert.Equal(t, tc.sampleRelease, options.SampleRelease)
				assert.Equal(t, tc.testIDOption, options.TestIDOptions[0])
				assert.Equal(t, tc.variantOption, options.VariantOption)
				assert.Equal(t, tc.advancedOption, options.AdvancedOption)
				assert.Equal(t, tc.cacheOption, options.CacheOption)
				assert.Equal(t, tc.includeAllTests, options.IncludeAllTests)
				if tc.errMessage != "" {
					assert.Error(t, err)
					assert.True(t, strings.Contains(err.Error(), tc.errMessage))
				}
			}
		})
	}

}

// TestHATEOASLinkCacheConsistency verifies that the cache key produced by the cache preloader
// (which resolves dates from a view) matches the cache key produced when a user follows a
// HATEOAS link (which embeds those dates as explicit query params). A mismatch means cache misses.
func TestHATEOASLinkCacheConsistency(t *testing.T) {
	roundingFactor := 12 * time.Hour
	roundingOffset := 4 * time.Hour

	releases := []v1.Release{
		{Release: "4.16", Status: "", GADate: util.DatePtr(2024, 6, 27, 0, 0, 0, 0, time.UTC)},
		{Release: "4.17", Status: "", GADate: util.DatePtr(2024, 12, 10, 0, 0, 0, 0, time.UTC)},
	}

	allJobVariants := crtest.JobVariants{Variants: map[string][]string{
		"Architecture":   {"amd64", "arm64"},
		"FeatureSet":     {"default", "techpreview"},
		"Installer":      {"ipi", "upi"},
		"LayeredProduct": {"none"},
		"Network":        {"ovn"},
		"Platform":       {"aws", "gcp"},
		"Suite":          {"unknown"},
		"Topology":       {"ha", "single"},
		"Upgrade":        {"micro", "minor", "none"},
	}}

	view := crview.View{
		Name: "4.17-main",
		BaseRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.16",
			},
			RelativeStart: "ga-30d",
			RelativeEnd:   "ga",
		},
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.17",
			},
			RelativeStart: "now-7d",
			RelativeEnd:   "now",
		},
		VariantOptions: reqopts.Variants{
			ColumnGroupBy:   defaultColumnGroupByVariants,
			DBGroupBy:       defaultDBGroupByVariants,
			IncludeVariants: includeVariants,
		},
		AdvancedOptions: reqopts.Advanced{
			MinimumFailure:   3,
			Confidence:       95,
			PityFactor:       5,
			IgnoreMissing:    false,
			IgnoreDisruption: true,
		},
	}
	views := []crview.View{view}

	// Step 1: Simulate the cache preloader path — resolve dates from the view
	baseReleaseOpts, err := utils.GetViewReleaseOptions(releases, "basis", view.BaseRelease, 0, 0)
	require.NoError(t, err)
	sampleReleaseOpts, err := utils.GetViewReleaseOptions(releases, "sample", view.SampleRelease, roundingFactor, roundingOffset)
	require.NoError(t, err)

	// Base release times must always be start-of-day / end-of-day, never TruncateAligned
	assert.Equal(t, 0, baseReleaseOpts.Start.Hour(), "base start must be 00:00 UTC (start-of-day)")
	assert.Equal(t, 0, baseReleaseOpts.Start.Minute(), "base start must be 00:00 UTC (start-of-day)")
	assert.Equal(t, 23, baseReleaseOpts.End.Hour(), "base end must be 23:59:59 UTC (end-of-day)")
	assert.Equal(t, 59, baseReleaseOpts.End.Minute(), "base end must be 23:59:59 UTC (end-of-day)")
	assert.Equal(t, 59, baseReleaseOpts.End.Second(), "base end must be 23:59:59 UTC (end-of-day)")

	preloaderKey := GeneratorCacheKey{
		BaseRelease:    baseReleaseOpts,
		SampleRelease:  sampleReleaseOpts,
		VariantOption:  view.VariantOptions,
		AdvancedOption: view.AdvancedOptions,
		TestIDOptions: []reqopts.TestIdentification{
			{
				TestID:            "openshift-tests:abc123",
				Component:         "TestComponent",
				Capability:        "TestCapability",
				RequestedVariants: map[string]string{"Architecture": "amd64", "Platform": "aws"},
			},
		},
	}

	// Step 2: Generate a HATEOAS link URL from those resolved dates (as the API would)
	hateoasURL, err := utils.GenerateTestDetailsURL(
		"openshift-tests:abc123",
		"https://sippy.example.com",
		view.Name,
		baseReleaseOpts,
		sampleReleaseOpts,
		view.AdvancedOptions,
		view.VariantOptions,
		reqopts.TestFilters{},
		"TestComponent",
		"TestCapability",
		[]string{"Architecture:amd64", "Platform:aws"},
		"",
	)
	require.NoError(t, err)

	// Step 3: Parse the HATEOAS URL as if a browser clicked it — this simulates the server
	// receiving the request and resolving options from URL params
	parsedURL, err := url.Parse(hateoasURL)
	require.NoError(t, err)
	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	require.NoError(t, err)
	parsedOpts, _, err := utils.ParseComponentReportRequest(views, releases, req, allJobVariants, roundingFactor, roundingOffset)
	require.NoError(t, err)

	requestKey := GeneratorCacheKey{
		BaseRelease:    parsedOpts.BaseRelease,
		SampleRelease:  parsedOpts.SampleRelease,
		VariantOption:  parsedOpts.VariantOption,
		AdvancedOption: parsedOpts.AdvancedOption,
		TestIDOptions:  parsedOpts.TestIDOptions,
	}

	// Step 4: The cache keys must match — if they don't, HATEOAS links will miss the cache
	preloaderJSON, err := json.Marshal(preloaderKey)
	require.NoError(t, err)
	requestJSON, err := json.Marshal(requestKey)
	require.NoError(t, err)

	assert.Equal(t, preloaderKey.BaseRelease, requestKey.BaseRelease,
		"base release mismatch between preloader and HATEOAS link request")
	assert.Equal(t, preloaderKey.SampleRelease, requestKey.SampleRelease,
		"sample release mismatch between preloader and HATEOAS link request")
	assert.JSONEq(t, string(preloaderJSON), string(requestJSON),
		fmt.Sprintf("cache key mismatch — preloader and HATEOAS link produce different keys.\nPreloader: %s\nRequest:   %s", preloaderJSON, requestJSON))
}
