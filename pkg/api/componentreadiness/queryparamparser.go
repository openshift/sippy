package componentreadiness

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/util"
)

// nolint:gocyclo
func ParseComponentReportRequest(
	views []crtype.View,
	req *http.Request,
	allJobVariants crtype.JobVariants,
	crTimeRoundingFactor time.Duration,
) (
	opts crtype.RequestOptions,
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
		opts.BaseRelease, err = GetViewReleaseOptions("basis", view.BaseRelease, crTimeRoundingFactor)
		if err != nil {
			return
		}
		opts.SampleRelease, err = GetViewReleaseOptions("sample", view.SampleRelease, crTimeRoundingFactor)
		if err != nil {
			return
		}
	} else {
		opts.BaseRelease.Release = req.URL.Query().Get("baseRelease")
		if opts.BaseRelease.Release == "" {
			err = fmt.Errorf("missing baseRelease")
			return
		}

		opts.SampleRelease.Release = req.URL.Query().Get("sampleRelease")
		if opts.SampleRelease.Release == "" {
			err = fmt.Errorf("missing sampleRelease")
			return
		}
		// We only support pull request jobs as the sample, not the basis:
		opts.SampleRelease.PullRequestOptions = parsePROptions(req)

		if opts.VariantOption, err = parseVariantOptions(req, allJobVariants); err != nil {
			return
		}
		if opts.AdvancedOption, err = parseAdvancedOptions(req); err != nil {
			return
		}

		// TODO: if specified, allow these to override view defaults for start/end time.
		// will need to relocate this outside this else.
		opts.BaseRelease, err = parseDateRange(req, opts.BaseRelease, "baseStartTime", "baseEndTime", crTimeRoundingFactor)
		if err != nil {
			return
		}
		opts.SampleRelease, err = parseDateRange(req, opts.SampleRelease, "sampleStartTime", "sampleEndTime", crTimeRoundingFactor)
		if err != nil {
			return
		}

	}

	// Params below this point can be used with and without views:

	opts.TestIDOption = crtype.RequestTestIdentificationOptions{
		Component:  req.URL.Query().Get("component"),
		Capability: req.URL.Query().Get("capability"),
		TestID:     req.URL.Query().Get("testId"),
	}

	opts.CacheOption.ForceRefresh, err = ParseBoolArg(req, "forceRefresh", false)
	if err != nil {
		return
	}
	opts.CacheOption.CRTimeRoundingFactor = crTimeRoundingFactor

	return
}

// getRequestedView returns the view requested per the view param, or nil if none.
func getRequestedView(req *http.Request, views []crtype.View) (*crtype.View, error) {
	viewRequested := req.URL.Query().Get("view")
	if viewRequested == "" {
		return nil, nil
	}

	// the following params are not compatible with use of a view and will generate an error if combined with one:
	incompatible := []string{
		"baseRelease", "sampleRelease", // release opts
		"samplePROrg", "samplePRRepo", "samplePRNumber", // PR opts
		"columnGroupBy", "dbGroupBy", // grouping
		"includeVariant", // variants
		"confidence", "pity", "minFail",
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
	releaseType string,
	viewRelease crtype.RequestRelativeReleaseOptions,
	roundingFactor time.Duration,
) (crtype.RequestReleaseOptions, error) {

	var err error
	opts := crtype.RequestReleaseOptions{Release: viewRelease.Release}
	opts.Start, err = util.ParseCRReleaseTime(opts.Release, viewRelease.RelativeStart, true, roundingFactor)
	if err != nil {
		return opts, fmt.Errorf("%s start time %q in wrong format: %v", releaseType, viewRelease.RelativeStart, err)
	}
	opts.End, err = util.ParseCRReleaseTime(opts.Release, viewRelease.RelativeEnd, false, roundingFactor)
	if err != nil {
		return opts, fmt.Errorf("%s start time %q in wrong format: %v", releaseType, viewRelease.RelativeEnd, err)
	}
	return opts, nil
}

func parsePROptions(req *http.Request) *crtype.PullRequestOptions {
	pro := crtype.PullRequestOptions{
		Org:      req.URL.Query().Get("samplePROrg"),
		Repo:     req.URL.Query().Get("samplePRRepo"),
		PRNumber: req.URL.Query().Get("samplePRNumber"),
	}
	if pro.Org == "" || pro.Repo == "" || pro.PRNumber == "" {
		return nil
	}
	return &pro
}

func parseVariantOptions(req *http.Request, allJobVariants crtype.JobVariants) (opts crtype.RequestVariantOptions, err error) {
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

	opts.RequestedVariants = map[string]string{}
	// Only the dbGroupBy variants can be specifically requested
	for _, variant := range opts.DBGroupBy.List() {
		if value := req.URL.Query().Get(variant); value != "" {
			opts.RequestedVariants[variant] = value
		}
	}
	includeVariants := req.URL.Query()["includeVariant"]
	opts.IncludeVariants, err = api.VariantListToMap(allJobVariants, includeVariants)
	if err != nil {
		return
	}
	compareVariants, err := api.VariantListToMap(allJobVariants, req.URL.Query()["compareVariant"])
	if err != nil {
		return
	}

	opts.VariantCrossCompare = req.URL.Query()["variantCrossCompare"]
	if len(opts.VariantCrossCompare) > 0 {
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
	param := req.URL.Query().Get(name)
	if param == "" {
		return defaultVal, nil
	}
	val, err := strconv.Atoi(param)
	if err != nil {
		return val, fmt.Errorf(name + " is not an integer")
	}
	if !validator(val) {
		return val, fmt.Errorf("confidence is not in the correct range")
	}
	return val, nil
}

func ParseBoolArg(req *http.Request, name string, defaultVal bool) (bool, error) {
	param := req.URL.Query().Get(name)
	if param == "" {
		return defaultVal, nil
	}
	val, err := strconv.ParseBool(param)
	if err != nil {
		return val, fmt.Errorf(name + " is not a boolean")
	}
	return val, nil
}

func parseAdvancedOptions(req *http.Request) (advancedOption crtype.RequestAdvancedOptions, err error) {
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

	advancedOption.IgnoreMissing, err = ParseBoolArg(req, "ignoreMissing", false)
	if err != nil {
		return advancedOption, err
	}

	advancedOption.IgnoreDisruption, err = ParseBoolArg(req, "ignoreDisruption", true)
	if err != nil {
		return advancedOption, err
	}
	return
}

func parseDateRange(req *http.Request,
	releaseOpts crtype.RequestReleaseOptions,
	startName string, endName string,
	roundingFactor time.Duration,
) (crtype.RequestReleaseOptions, error) {
	var err error

	timeStr := req.URL.Query().Get(startName)
	releaseOpts.Start, err = util.ParseCRReleaseTime(releaseOpts.Release, timeStr, true, roundingFactor)
	if err != nil {
		return releaseOpts, fmt.Errorf(startName + " in wrong format")
	}

	timeStr = req.URL.Query().Get(endName)
	releaseOpts.End, err = util.ParseCRReleaseTime(releaseOpts.Release, timeStr, false, roundingFactor)
	if err != nil {
		return releaseOpts, fmt.Errorf(endName + " in wrong format")
	}
	return releaseOpts, nil
}
