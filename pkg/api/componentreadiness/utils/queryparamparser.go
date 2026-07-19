package utils

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/param"
	"k8s.io/apimachinery/pkg/util/sets"
)

func ParseComponentReportRequest(
	views []crview.View,
	releases []v1.Release,
	req *http.Request,
	allJobVariants crtest.JobVariants,
	crTimeRoundingFactor, crTimeRoundingOffset time.Duration,
) (
	opts reqopts.RequestOptions,
	warnings []string,
	err error,
) {
	view, err := getRequestedView(req, views)
	if err != nil {
		return
	}
	if view != nil {
		opts, warnings, err = applyViewDefaults(view, releases, allJobVariants, crTimeRoundingFactor, crTimeRoundingOffset)
		if err != nil {
			return
		}
	}

	if err = parseRequiredReleases(req, view, &opts); err != nil {
		return
	}

	if prOpts := parsePROptions(req); prOpts != nil {
		opts.SampleRelease.PullRequestOptions = prOpts
	}
	if payloadOpts := parsePayloadOptions(req); payloadOpts != nil {
		opts.SampleRelease.PayloadOptions = payloadOpts
	}

	if testCaps := req.URL.Query()["testCapabilities"]; len(testCaps) > 0 {
		opts.Capabilities = testCaps
	}
	if testLifecycles := req.URL.Query()["testLifecycles"]; len(testLifecycles) > 0 {
		opts.Lifecycles = testLifecycles
	}

	variantOpts, vWarnings, vErr := parseVariantOptions(req, allJobVariants)
	if vErr != nil {
		return opts, warnings, vErr
	}
	warnings = append(warnings, vWarnings...)
	opts.VariantOption = mergeVariantOptions(req, view != nil, opts.VariantOption, variantOpts)

	advOpts, advErr := parseAdvancedOptions(req)
	if advErr != nil {
		return opts, warnings, advErr
	}
	opts.AdvancedOption = mergeAdvancedOptions(req, view != nil, opts.AdvancedOption, advOpts)

	if hasDateRangeInURL(req, "baseStartTime", "baseEndTime") {
		opts.BaseRelease, err = parseDateRange(releases, req, opts.BaseRelease, "baseStartTime", "baseEndTime", 0, 0)
		if err != nil {
			return
		}
	}
	if hasDateRangeInURL(req, "sampleStartTime", "sampleEndTime") {
		opts.SampleRelease, err = parseDateRange(releases, req, opts.SampleRelease, "sampleStartTime", "sampleEndTime", crTimeRoundingFactor, crTimeRoundingOffset)
		if err != nil {
			return
		}
	}

	opts.TestIDOptions = parseTestIDOptions(req, releases, opts.BaseRelease, opts.AdvancedOption, opts.VariantOption)

	opts.IncludeAllTests, err = ParseBoolArg(req, "includeAllTests", false)
	if err != nil {
		return
	}

	opts.CacheOption = cache.NewStandardCROptions(crTimeRoundingFactor, crTimeRoundingOffset)
	opts.CacheOption.ForceRefresh, err = ParseBoolArg(req, "forceRefresh", false)
	if err != nil {
		return
	}

	return
}

func applyViewDefaults(
	view *crview.View,
	releases []v1.Release,
	allJobVariants crtest.JobVariants,
	crTimeRoundingFactor, crTimeRoundingOffset time.Duration,
) (opts reqopts.RequestOptions, warnings []string, err error) {
	opts.ViewName = view.Name
	opts.VariantOption = view.VariantOptions
	opts.AdvancedOption = view.AdvancedOptions
	opts.TestFilters = view.TestFilters
	opts.BaseRelease, err = GetViewReleaseOptions(releases, "basis", view.BaseRelease, 0, 0)
	if err != nil {
		return
	}
	opts.SampleRelease, err = GetViewReleaseOptions(releases, "sample", view.SampleRelease, crTimeRoundingFactor, crTimeRoundingOffset)
	if err != nil {
		return
	}
	if opts.VariantOption.IncludeVariants != nil {
		viewWarnings := api.ValidateVariants(allJobVariants, opts.VariantOption.IncludeVariants, " from view")
		warnings = append(warnings, viewWarnings...)
	}
	return
}

func parseRequiredReleases(req *http.Request, view *crview.View, opts *reqopts.RequestOptions) error {
	if baseRelease := param.SafeRead(req, "baseRelease"); baseRelease != "" {
		opts.BaseRelease.Name = baseRelease
	} else if view == nil {
		return &api.ValidationError{Message: "missing baseRelease"}
	}
	if sampleRelease := param.SafeRead(req, "sampleRelease"); sampleRelease != "" {
		opts.SampleRelease.Name = sampleRelease
	} else if view == nil {
		return &api.ValidationError{Message: "missing sampleRelease"}
	}
	return nil
}

func mergeVariantOptions(req *http.Request, hasView bool, viewOpts, parsedOpts reqopts.Variants) reqopts.Variants {
	if !hasView {
		return parsedOpts
	}
	if req.URL.Query().Get("columnGroupBy") != "" {
		viewOpts.ColumnGroupBy = parsedOpts.ColumnGroupBy
	}
	if req.URL.Query().Get("dbGroupBy") != "" {
		viewOpts.DBGroupBy = parsedOpts.DBGroupBy
	}
	if len(req.URL.Query()["includeVariant"]) > 0 {
		viewOpts.IncludeVariants = parsedOpts.IncludeVariants
	}
	if len(req.URL.Query()["compareVariant"]) > 0 || len(req.URL.Query()["variantCrossCompare"]) > 0 {
		viewOpts.CompareVariants = parsedOpts.CompareVariants
		viewOpts.VariantCrossCompare = parsedOpts.VariantCrossCompare
	}
	return viewOpts
}

func mergeAdvancedOptions(req *http.Request, hasView bool, viewOpts, parsedOpts reqopts.Advanced) reqopts.Advanced {
	if !hasView {
		return parsedOpts
	}
	if req.URL.Query().Get("confidence") != "" {
		viewOpts.Confidence = parsedOpts.Confidence
	}
	if req.URL.Query().Get("pity") != "" {
		viewOpts.PityFactor = parsedOpts.PityFactor
	}
	if req.URL.Query().Get("minFail") != "" {
		viewOpts.MinimumFailure = parsedOpts.MinimumFailure
	}
	if req.URL.Query().Get("passRateNewTests") != "" {
		viewOpts.PassRateRequiredNewTests = parsedOpts.PassRateRequiredNewTests
	}
	if req.URL.Query().Get("passRateAllTests") != "" {
		viewOpts.PassRateRequiredAllTests = parsedOpts.PassRateRequiredAllTests
	}
	if req.URL.Query().Get("ignoreMissing") != "" {
		viewOpts.IgnoreMissing = parsedOpts.IgnoreMissing
	}
	if req.URL.Query().Get("ignoreDisruption") != "" {
		viewOpts.IgnoreDisruption = parsedOpts.IgnoreDisruption
	}
	if req.URL.Query().Get("flakeAsFailure") != "" {
		viewOpts.FlakeAsFailure = parsedOpts.FlakeAsFailure
	}
	if req.URL.Query().Get("includeMultiReleaseAnalysis") != "" {
		viewOpts.IncludeMultiReleaseAnalysis = parsedOpts.IncludeMultiReleaseAnalysis
	}
	if len(req.URL.Query()["keyTestName"]) > 0 {
		viewOpts.KeyTestNames = parsedOpts.KeyTestNames
	}
	return viewOpts
}

// parseTestIDOptions builds the test identification from URL params, including
// multi-release basis override and per-variant filtering.
func parseTestIDOptions(
	req *http.Request,
	releases []v1.Release,
	baseRelease reqopts.Release,
	advancedOption reqopts.Advanced,
	variantOption reqopts.Variants,
) []reqopts.TestIdentification {
	// TODO: leave nil for safer cache keys if params not set, sync with metrics and primecache.go
	// TODO: unit test that metrics and primecache cache keys match a request object here
	tid := reqopts.TestIdentification{
		Component:  req.URL.Query().Get("component"),
		Capability: req.URL.Query().Get("capability"),
		TestID:     req.URL.Query().Get("testId"),
	}
	if advancedOption.IncludeMultiReleaseAnalysis {
		testBasisRelease := param.SafeRead(req, "testBasisRelease")
		if len(testBasisRelease) > 0 && releases != nil {
			for _, release := range releases {
				if release.Release == testBasisRelease {
					if baseRelease.Name != testBasisRelease {
						tid.BaseOverrideRelease = testBasisRelease
					}
					break
				}
			}
		}
	}
	tid.RequestedVariants = map[string]string{}
	for _, variant := range sets.List(variantOption.DBGroupBy) {
		if value := req.URL.Query().Get(variant); value != "" {
			tid.RequestedVariants[variant] = value
		}
	}
	return []reqopts.TestIdentification{tid}
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
	return nil, &api.ValidationError{Message: fmt.Sprintf("unknown view: %s", viewRequested)}
}

// Translate relative start/end times to actual time.Time:
func GetViewReleaseOptions(
	releases []v1.Release,
	releaseType string,
	viewRelease reqopts.RelativeRelease,
	roundingFactor, roundingOffset time.Duration,
) (reqopts.Release, error) {

	var err error
	opts := reqopts.Release{Name: viewRelease.Name}
	opts.Start, err = util.ParseCRReleaseTime(releases, opts.Name, viewRelease.RelativeStart, true, nil, roundingFactor, roundingOffset)
	if err != nil {
		return opts, &api.ValidationError{Message: fmt.Sprintf("%s start time %q in wrong format: %v", releaseType, viewRelease.RelativeStart, err)}
	}
	opts.End, err = util.ParseCRReleaseTime(releases, opts.Name, viewRelease.RelativeEnd, false, nil, roundingFactor, roundingOffset)
	if err != nil {
		return opts, &api.ValidationError{Message: fmt.Sprintf("%s end time %q in wrong format: %v", releaseType, viewRelease.RelativeEnd, err)}
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

func parseVariantOptions(req *http.Request, allJobVariants crtest.JobVariants) (opts reqopts.Variants, warnings []string, err error) {
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

	compareVariants, err := api.VariantListToMap(allJobVariants, req.URL.Query()["compareVariant"])
	if err != nil {
		return
	}

	opts.VariantCrossCompare = req.URL.Query()["variantCrossCompare"]
	if len(opts.VariantCrossCompare) > 0 {
		opts.CompareVariants = map[string][]string{}
		for group, variants := range opts.IncludeVariants {
			opts.CompareVariants[group] = variants
		}

		for _, group := range opts.VariantCrossCompare {
			if variants := compareVariants[group]; len(variants) > 0 {
				opts.CompareVariants[group] = variants
			} else {
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
		return val, &api.ValidationError{Message: name + " is not an integer"}
	}
	if !validator(val) {
		return val, &api.ValidationError{Message: name + " is not in the correct range"}
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
		return val, &api.ValidationError{Message: name + " is not a boolean"}
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
	roundingFactor, roundingOffset time.Duration,
) (reqopts.Release, error) {
	var err error

	timeStr := req.URL.Query().Get(startName)
	releaseOpts.Start, err = util.ParseCRReleaseTime(allReleases, releaseOpts.Name, timeStr, true, nil, roundingFactor, roundingOffset)
	if err != nil {
		return releaseOpts, &api.ValidationError{Message: startName + " in wrong format"}
	}

	timeStr = req.URL.Query().Get(endName)
	releaseOpts.End, err = util.ParseCRReleaseTime(allReleases, releaseOpts.Name, timeStr, false, nil, roundingFactor, roundingOffset)
	if err != nil {
		return releaseOpts, &api.ValidationError{Message: endName + " in wrong format"}
	}
	return releaseOpts, nil
}

// hasDateRangeInURL checks if date range parameters are provided in the URL
func hasDateRangeInURL(req *http.Request, startParam, endParam string) bool {
	return req.URL.Query().Get(startParam) != "" || req.URL.Query().Get(endParam) != ""
}
