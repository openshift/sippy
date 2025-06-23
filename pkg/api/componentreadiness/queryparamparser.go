package componentreadiness

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/param"
)

// nolint:gocyclo
func ParseComponentReportRequest(
	views []crview.View,
	releases []v1.Release,
	req *http.Request,
	allJobVariants crtest.JobVariants,
	crTimeRoundingFactor time.Duration,
	overrides []configv1.VariantJunitTableOverride,
) (
	opts reqopts.RequestOptions,
	err error,
) {
	// Check if the user specified a view, in which case only some query params can be used.
	view, err := getRequestedView(req, views)
	if err != nil {
		return
	}

	if view != nil {
		// set params from view
		opts.VariantOption = view.VariantOptions
		opts.AdvancedOption = view.AdvancedOptions
		opts.BaseRelease, err = GetViewReleaseOptions(releases, "basis", view.BaseRelease, crTimeRoundingFactor)
		if err != nil {
			return
		}
		opts.SampleRelease, err = GetViewReleaseOptions(releases, "sample", view.SampleRelease, crTimeRoundingFactor)
		if err != nil {
			return
		}
	} else {
		opts.BaseRelease.Name = param.SafeRead(req, "baseRelease")
		if opts.BaseRelease.Name == "" {
			err = fmt.Errorf("missing baseRelease")
			return
		}

		opts.SampleRelease.Name = param.SafeRead(req, "sampleRelease")
		if opts.SampleRelease.Name == "" {
			err = fmt.Errorf("missing sampleRelease")
			return
		}
		// We only support pull request and payload jobs as the sample, not the basis:
		opts.SampleRelease.PullRequestOptions = parsePROptions(req)
		opts.SampleRelease.PayloadOptions = parsePayloadOptions(req)

		if opts.VariantOption, err = parseVariantOptions(req, allJobVariants, overrides); err != nil {
			return
		}
		if opts.AdvancedOption, err = parseAdvancedOptions(req); err != nil {
			return
		}

		// TODO: if specified, allow these to override view defaults for start/end time.
		// will need to relocate this outside this else.
		opts.BaseRelease, err = parseDateRange(releases, req, opts.BaseRelease, "baseStartTime", "baseEndTime", crTimeRoundingFactor)
		if err != nil {
			return
		}
		opts.SampleRelease, err = parseDateRange(releases, req, opts.SampleRelease, "sampleStartTime", "sampleEndTime", crTimeRoundingFactor)
		if err != nil {
			return
		}

	}

	// Params below this point can be used with and without views:
	// TODO: leave nil for safer cache keys if params not set, sync with metrics and primecache.go
	// TODO: unit test that metrics and primecache cache keys match a request object here
	opts.TestIDOptions = []reqopts.TestIdentification{
		{
			// these are semi-freeform and only used in lookup keys, so don't need validation
			Component:  req.URL.Query().Get("component"),
			Capability: req.URL.Query().Get("capability"),
			TestID:     req.URL.Query().Get("testId"),
		},
	}
	if opts.AdvancedOption.IncludeMultiReleaseAnalysis {
		// check to see if we have an individual test which is using a fallback release for basis
		testBasisRelease := param.SafeRead(req, "testBasisRelease")
		if len(testBasisRelease) > 0 && releases != nil {
			// indicates we fell back to a previous release
			// get that release and find the dates associated with it.
			for _, release := range releases {
				if release.Release == testBasisRelease {
					// found the release so update if not already set
					// if it is already the base release we don't update
					// change dates
					if opts.BaseRelease.Name != testBasisRelease {
						opts.TestIDOptions[0].BaseOverrideRelease = testBasisRelease
					}
					break
				}
			}
		}
	}
	opts.TestIDOptions[0].RequestedVariants = map[string]string{}
	// Only the dbGroupBy variants can be specifically requested
	for _, variant := range opts.VariantOption.DBGroupBy.List() {
		if value := req.URL.Query().Get(variant); value != "" {
			opts.TestIDOptions[0].RequestedVariants[variant] = value
		}
	}

	opts.CacheOption.ForceRefresh, err = ParseBoolArg(req, "forceRefresh", false)
	if err != nil {
		return
	}
	opts.CacheOption.CRTimeRoundingFactor = crTimeRoundingFactor

	return
}

// getRequestedView returns the view requested per the view param, or nil if none.
func getRequestedView(req *http.Request, views []crview.View) (*crview.View, error) {
	viewRequested := req.URL.Query().Get("view") // used only to lookup by view name
	if viewRequested == "" {
		return nil, nil
	}

	// the following params are not compatible with use of a view and will generate an error if combined with one:
	incompatible := []string{
		"baseRelease", "sampleRelease", // release opts
		"samplePROrg", "samplePRRepo", "samplePRNumber", // PR opts
		"columnGroupBy", "dbGroupBy", // grouping
		"samplePayloadTag",                                        // Payload opts
		"includeVariant", "compareVariant", "variantCrossCompare", // variants
		"confidence", "pity", "minFail", "passRateNewTests", "passRateAllTests",
		"ignoreMissing", "ignoreDisruption", // advanced opts
	}
	found := []string{}
	for _, p := range incompatible {
		if req.URL.Query().Get(p) != "" {
			found = append(found, p)
		}
	}
	if len(found) > 0 {
		return nil, fmt.Errorf("params cannot be combined with view: %v", found)
	}

	// find the requested view name among our known views:
	for _, view := range views {
		if view.Name == viewRequested {
			return &view, nil
		}
	}
	return nil, fmt.Errorf("unknown view: %s", viewRequested)
}

// Translate relative start/end times to actual time.Time:
func GetViewReleaseOptions(
	releases []v1.Release,
	releaseType string,
	viewRelease reqopts.RelativeRelease,
	roundingFactor time.Duration,
) (reqopts.Release, error) {

	var err error
	opts := reqopts.Release{Name: viewRelease.Name}
	opts.Start, err = util.ParseCRReleaseTime(releases, opts.Name, viewRelease.RelativeStart, true, nil, roundingFactor)
	if err != nil {
		return opts, fmt.Errorf("%s start time %q in wrong format: %v", releaseType, viewRelease.RelativeStart, err)
	}
	opts.End, err = util.ParseCRReleaseTime(releases, opts.Name, viewRelease.RelativeEnd, false, nil, roundingFactor)
	if err != nil {
		return opts, fmt.Errorf("%s start time %q in wrong format: %v", releaseType, viewRelease.RelativeEnd, err)
	}
	return opts, nil
}

func parsePROptions(req *http.Request) *reqopts.PullRequest {
	pro := reqopts.PullRequest{
		Org:      param.SafeRead(req, "samplePROrg"),
		Repo:     param.SafeRead(req, "samplePRRepo"),
		PRNumber: param.SafeRead(req, "samplePRNumber"),
	}
	if pro.Org == "" || pro.Repo == "" || pro.PRNumber == "" {
		return nil
	}
	return &pro
}

func parsePayloadOptions(req *http.Request) *reqopts.Payload {
	po := reqopts.Payload{
		Tag: param.SafeRead(req, "samplePayloadTag"),
	}
	if po.Tag == "" {
		return nil
	}
	return &po
}

func parseVariantOptions(req *http.Request, allJobVariants crtest.JobVariants, overrides []configv1.VariantJunitTableOverride) (opts reqopts.Variants, err error) {
	columnGroupBy := req.URL.Query().Get("columnGroupBy")
	opts.ColumnGroupBy, err = api.VariantsStringToSet(allJobVariants, columnGroupBy)
	if err != nil {
		return
	}
	dbGroupBy := req.URL.Query().Get("dbGroupBy")
	opts.DBGroupBy, err = api.VariantsStringToSet(allJobVariants, dbGroupBy)
	if err != nil {
		return
	}

	includeVariants := req.URL.Query()["includeVariant"]
	opts.IncludeVariants, err = api.VariantListToMap(allJobVariants, includeVariants)
	if err != nil {
		return
	}

	// check if any included variants have a junit table override:
	var overriddenVariant string
	for _, or := range overrides {
		if containsOverriddenVariant(opts.IncludeVariants, or.VariantName, or.VariantValue) {
			overriddenVariant = fmt.Sprintf("%s=%s", or.VariantName, or.VariantValue)
			break
		}
	}

	compareVariants, err := api.VariantListToMap(allJobVariants, req.URL.Query()["compareVariant"])
	if err != nil {
		return
	}

	opts.VariantCrossCompare = req.URL.Query()["variantCrossCompare"]
	if len(opts.VariantCrossCompare) > 0 {

		// cross compare is not supported with variant overrides
		if len(overriddenVariant) > 0 {
			err = fmt.Errorf("variant cross compare is not supported with overridden variant: %s", overriddenVariant)
			return
		}

		// when we are cross-comparing variants, we need to construct the compareVariants map from the parameters.
		// the resulting compareVariants map is includeVariants...
		opts.CompareVariants = map[string][]string{}
		for group, variants := range opts.IncludeVariants {
			opts.CompareVariants[group] = variants
		}

		// ...with overrides from compareVariant parameters.
		for _, group := range opts.VariantCrossCompare {
			if variants := compareVariants[group]; len(variants) > 0 {
				opts.CompareVariants[group] = variants
			} else {
				// a group override without any variants listed means not to restrict the variants in this group.
				// in that case we don't want any where clause for the group, so we just omit it from the map.
				delete(opts.CompareVariants, group)
			}
		}
	}
	return
}

func ParseIntArg(req *http.Request, name string, defaultVal int, validator func(int) bool) (int, error) {
	valueStr := req.URL.Query().Get(name)
	if valueStr == "" {
		return defaultVal, nil
	}
	val, err := strconv.Atoi(valueStr)
	if err != nil {
		return val, errors.New(name + " is not an integer")
	}
	if !validator(val) {
		return val, errors.New("confidence is not in the correct range")
	}
	return val, nil
}

func ParseBoolArg(req *http.Request, name string, defaultVal bool) (bool, error) {
	valueStr := req.URL.Query().Get(name)
	if valueStr == "" {
		return defaultVal, nil
	}
	val, err := strconv.ParseBool(valueStr)
	if err != nil {
		return val, errors.New(name + " is not a boolean")
	}
	return val, nil
}

func parseAdvancedOptions(req *http.Request) (advancedOption reqopts.Advanced, err error) {
	advancedOption.Confidence, err = ParseIntArg(req, "confidence", 95,
		func(v int) bool { return v >= 0 && v <= 100 })
	if err != nil {
		return advancedOption, err
	}

	advancedOption.PityFactor, err = ParseIntArg(req, "pity", 5,
		func(v int) bool { return v >= 0 && v <= 100 })
	if err != nil {
		return advancedOption, err
	}

	advancedOption.MinimumFailure, err = ParseIntArg(req, "minFail", 3,
		func(v int) bool { return v >= 0 })
	if err != nil {
		return advancedOption, err
	}

	advancedOption.PassRateRequiredNewTests, err = ParseIntArg(req, "passRateNewTests", 0,
		func(v int) bool { return v >= 0 && v <= 100 })
	if err != nil {
		return advancedOption, err
	}

	advancedOption.PassRateRequiredAllTests, err = ParseIntArg(req, "passRateAllTests", 0,
		func(v int) bool { return v >= 0 && v <= 100 })
	if err != nil {
		return advancedOption, err
	}

	advancedOption.IgnoreMissing, err = ParseBoolArg(req, "ignoreMissing", false)
	if err != nil {
		return advancedOption, err
	}

	advancedOption.IgnoreDisruption, err = ParseBoolArg(req, "ignoreDisruption", true)
	if err != nil {
		return advancedOption, err
	}

	advancedOption.FlakeAsFailure, err = ParseBoolArg(req, "flakeAsFailure", false)
	if err != nil {
		return advancedOption, err
	}

	advancedOption.IncludeMultiReleaseAnalysis, err = ParseBoolArg(req, "includeMultiReleaseAnalysis", false)
	if err != nil {
		return advancedOption, err
	}

	return
}

func parseDateRange(allReleases []v1.Release, req *http.Request,
	releaseOpts reqopts.Release,
	startName string, endName string,
	roundingFactor time.Duration,
) (reqopts.Release, error) {
	var err error

	timeStr := req.URL.Query().Get(startName)
	releaseOpts.Start, err = util.ParseCRReleaseTime(allReleases, releaseOpts.Name, timeStr, true, nil, roundingFactor)
	if err != nil {
		return releaseOpts, errors.New(startName + " in wrong format")
	}

	timeStr = req.URL.Query().Get(endName)
	releaseOpts.End, err = util.ParseCRReleaseTime(allReleases, releaseOpts.Name, timeStr, false, nil, roundingFactor)
	if err != nil {
		return releaseOpts, errors.New(endName + " in wrong format")
	}
	return releaseOpts, nil
}
