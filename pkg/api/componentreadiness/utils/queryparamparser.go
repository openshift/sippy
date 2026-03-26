package utils

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
	warnings []string,
	err error,
) {
	// Check if the user specified a view, which provides defaults that can be overridden by URL params
	view, err := getRequestedView(req, views)
	if err != nil {
		return
	}

	// Start with view defaults if provided
	if view != nil {
		opts.ViewName = view.Name
		opts.VariantOption = view.VariantOptions
		opts.AdvancedOption = view.AdvancedOptions
		opts.TestFilters = view.TestFilters
		opts.BaseRelease, err = GetViewReleaseOptions(releases, "basis", view.BaseRelease, crTimeRoundingFactor)
		if err != nil {
			return
		}
		opts.SampleRelease, err = GetViewReleaseOptions(releases, "sample", view.SampleRelease, crTimeRoundingFactor)
		if err != nil {
			return
		}

		// Validate variants from the view's include_variants
		if opts.VariantOption.IncludeVariants != nil {
			viewWarnings := api.ValidateVariants(allJobVariants, opts.VariantOption.IncludeVariants, " from view")
			warnings = append(warnings, viewWarnings...)
		}
	}

	// Parse URL parameters - these override view defaults if view was provided
	// If no view, these are required parameters
	if baseRelease := param.SafeRead(req, "baseRelease"); baseRelease != "" {
		opts.BaseRelease.Name = baseRelease
	} else if view == nil {
		err = fmt.Errorf("missing baseRelease")
		return
	}

	if sampleRelease := param.SafeRead(req, "sampleRelease"); sampleRelease != "" {
		opts.SampleRelease.Name = sampleRelease
	} else if view == nil {
		err = fmt.Errorf("missing sampleRelease")
		return
	}

	// PR and Payload options override view defaults
	if prOpts := parsePROptions(req); prOpts != nil {
		opts.SampleRelease.PullRequestOptions = prOpts
	}
	if payloadOpts := parsePayloadOptions(req); payloadOpts != nil {
		opts.SampleRelease.PayloadOptions = payloadOpts
	}

	// Test filters override view defaults
	if testCaps := req.URL.Query()["testCapabilities"]; len(testCaps) > 0 {
		opts.TestFilters.Capabilities = testCaps
	}
	if testLifecycles := req.URL.Query()["testLifecycles"]; len(testLifecycles) > 0 {
		opts.TestFilters.Lifecycles = testLifecycles
	}

	// Variant options - merge with view defaults
	variantOpts, vWarnings, vErr := parseVariantOptions(req, allJobVariants, overrides)
	if vErr != nil {
		err = vErr
		return
	}
	warnings = append(warnings, vWarnings...)
	if view != nil {
		// Merge: override individual fields from URL while preserving view defaults
		if req.URL.Query().Get("columnGroupBy") != "" {
			opts.VariantOption.ColumnGroupBy = variantOpts.ColumnGroupBy
		}
		if req.URL.Query().Get("dbGroupBy") != "" {
			opts.VariantOption.DBGroupBy = variantOpts.DBGroupBy
		}
		if len(req.URL.Query()["includeVariant"]) > 0 {
			opts.VariantOption.IncludeVariants = variantOpts.IncludeVariants
		}
		if len(req.URL.Query()["compareVariant"]) > 0 || len(req.URL.Query()["variantCrossCompare"]) > 0 {
			// CompareVariants and VariantCrossCompare are related, update together
			opts.VariantOption.CompareVariants = variantOpts.CompareVariants
			opts.VariantOption.VariantCrossCompare = variantOpts.VariantCrossCompare
		}
	} else {
		opts.VariantOption = variantOpts
	}

	// Advanced options - merge with view defaults
	advOpts, advErr := parseAdvancedOptions(req)
	if advErr != nil {
		err = advErr
		return
	}
	if view != nil {
		// Merge: only override fields that were explicitly provided in URL
		if req.URL.Query().Get("confidence") != "" {
			opts.AdvancedOption.Confidence = advOpts.Confidence
		}
		if req.URL.Query().Get("pity") != "" {
			opts.AdvancedOption.PityFactor = advOpts.PityFactor
		}
		if req.URL.Query().Get("minFail") != "" {
			opts.AdvancedOption.MinimumFailure = advOpts.MinimumFailure
		}
		if req.URL.Query().Get("passRateNewTests") != "" {
			opts.AdvancedOption.PassRateRequiredNewTests = advOpts.PassRateRequiredNewTests
		}
		if req.URL.Query().Get("passRateAllTests") != "" {
			opts.AdvancedOption.PassRateRequiredAllTests = advOpts.PassRateRequiredAllTests
		}
		if req.URL.Query().Get("ignoreMissing") != "" {
			opts.AdvancedOption.IgnoreMissing = advOpts.IgnoreMissing
		}
		if req.URL.Query().Get("ignoreDisruption") != "" {
			opts.AdvancedOption.IgnoreDisruption = advOpts.IgnoreDisruption
		}
		if req.URL.Query().Get("flakeAsFailure") != "" {
			opts.AdvancedOption.FlakeAsFailure = advOpts.FlakeAsFailure
		}
		if req.URL.Query().Get("includeMultiReleaseAnalysis") != "" {
			opts.AdvancedOption.IncludeMultiReleaseAnalysis = advOpts.IncludeMultiReleaseAnalysis
		}
		if len(req.URL.Query()["keyTestName"]) > 0 {
			opts.AdvancedOption.KeyTestNames = advOpts.KeyTestNames
		}
	} else {
		opts.AdvancedOption = advOpts
	}

	// Date ranges override view defaults
	if hasDateRangeInURL(req, "baseStartTime", "baseEndTime") {
		opts.BaseRelease, err = parseDateRange(releases, req, opts.BaseRelease, "baseStartTime", "baseEndTime", crTimeRoundingFactor)
		if err != nil {
			return
		}
	}
	if hasDateRangeInURL(req, "sampleStartTime", "sampleEndTime") {
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
// The view provides defaults that can be overridden by URL parameters.
func getRequestedView(req *http.Request, views []crview.View) (*crview.View, error) {
	viewRequested := req.URL.Query().Get("view") // used only to lookup by view name
	if viewRequested == "" {
		return nil, nil
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
	tags := req.URL.Query()["samplePayloadTag"]
	if len(tags) == 0 {
		return nil
	}
	return &reqopts.Payload{
		Tags: tags,
	}
}

func parseVariantOptions(req *http.Request, allJobVariants crtest.JobVariants, overrides []configv1.VariantJunitTableOverride) (opts reqopts.Variants, warnings []string, err error) {
	warnings = []string{}
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
	var includeWarnings []string
	opts.IncludeVariants, includeWarnings, err = api.VariantListToMapWithWarnings(allJobVariants, includeVariants)
	if err != nil {
		return
	}
	warnings = append(warnings, includeWarnings...)

	// check if any included variants have a junit table override:
	var overriddenVariant string
	for _, or := range overrides {
		if ContainsOverriddenVariant(opts.IncludeVariants, or.VariantName, or.VariantValue) {
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

	// Parse key test names - these are tests that when they fail in a job,
	// all other test failures in that job are excluded from regression analysis
	advancedOption.KeyTestNames = req.URL.Query()["keyTestName"]

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

// hasDateRangeInURL checks if date range parameters are provided in the URL
func hasDateRangeInURL(req *http.Request, startParam, endParam string) bool {
	return req.URL.Query().Get(startParam) != "" || req.URL.Query().Get(endParam) != ""
}
