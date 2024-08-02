package componentreadiness

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/util"
	"github.com/pkg/errors"
)

func ParseComponentReportRequest(
	views []apitype.ComponentReportView,
	req *http.Request,
	allJobVariants apitype.JobVariants,
	crTimeRoundingFactor time.Duration) (
	baseRelease apitype.ComponentReportRequestReleaseOptions,
	sampleRelease apitype.ComponentReportRequestReleaseOptions,
	testIDOption apitype.ComponentReportRequestTestIdentificationOptions,
	variantOption apitype.ComponentReportRequestVariantOptions,
	advancedOption apitype.ComponentReportRequestAdvancedOptions,
	cacheOption cache.RequestOptions,
	err error) {

	// Check if the user specified a view, in which case only some query params can be used.
	viewRequested := req.URL.Query().Get("view")
	var view *apitype.ComponentReportView
	if viewRequested != "" {
		for i, v := range views {
			if v.Name == viewRequested {
				view = &views[i]
				break
			}
		}
		if view == nil {
			err = fmt.Errorf("unknown view: %s", viewRequested)
			return
		}

		// the following params are not compatible with use of a view and will generate an error if combined with one:
		if pErr := anyParamSpecified(req,
			"baseRelease",
			"sampleRelease",
			"samplePROrg",
			"samplePRRepo",
			"samplePRNumber",
			"columnGroupBy",
			"dbGroupBy",
			"includeVariant",
			"confidence",
			"pity",
			"minFail",
			"ignoreMissing",
			"ignoreDisruption",
		); pErr != nil {
			err = pErr
			return
		}
		// set params from view
		variantOption = view.VariantOptions
		advancedOption = view.AdvancedOptions
		baseRelease = apitype.ComponentReportRequestReleaseOptions{
			Release: view.BaseRelease.Release,
		}
		sampleRelease = apitype.ComponentReportRequestReleaseOptions{
			Release: view.SampleRelease.Release,
		}
		// Translate relative start/end times to actual time.Time:
		baseRelease.Start, err = util.ParseCRReleaseTime(baseRelease.Release, view.BaseRelease.RelativeStart, true, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("base start time in wrong format")
			return
		}
		baseRelease.End, err = util.ParseCRReleaseTime(baseRelease.Release, view.BaseRelease.RelativeEnd, false, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("base end time in wrong format")
			return
		}
		sampleRelease.Start, err = util.ParseCRReleaseTime(sampleRelease.Release, view.SampleRelease.RelativeStart, true, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("sample start time in wrong format")
			return
		}
		sampleRelease.End, err = util.ParseCRReleaseTime(sampleRelease.Release, view.SampleRelease.RelativeEnd, false, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("sample end time in wrong format")
			return
		}
	} else {
		baseRelease.Release = req.URL.Query().Get("baseRelease")
		if baseRelease.Release == "" {
			err = fmt.Errorf("missing base_release")
			return
		}

		sampleRelease.Release = req.URL.Query().Get("sampleRelease")
		if sampleRelease.Release == "" {
			err = fmt.Errorf("missing sample_release")
			return
		}
		// We only support pull request jobs as the sample, not the basis:
		samplePROrg := req.URL.Query().Get("samplePROrg")
		samplePRRepo := req.URL.Query().Get("samplePRRepo")
		samplePRNumber := req.URL.Query().Get("samplePRNumber")
		if len(samplePROrg) > 0 && len(samplePRRepo) > 0 && len(samplePRNumber) > 0 {
			sampleRelease.PullRequestOptions = &apitype.PullRequestOptions{
				Org:      samplePROrg,
				Repo:     samplePRRepo,
				PRNumber: samplePRNumber,
			}
		}

		variantOption, err = parseVariantOptions(req, allJobVariants)
		if err != nil {
			return
		}

		advancedOption.Confidence = 95
		confidenceStr := req.URL.Query().Get("confidence")
		if confidenceStr != "" {
			advancedOption.Confidence, err = strconv.Atoi(confidenceStr)
			if err != nil {
				err = fmt.Errorf("confidence is not a number")
				return
			}
			if advancedOption.Confidence < 0 || advancedOption.Confidence > 100 {
				err = fmt.Errorf("confidence is not in the correct range")
				return
			}
		}

		advancedOption.PityFactor = 5
		pityStr := req.URL.Query().Get("pity")
		if pityStr != "" {
			advancedOption.PityFactor, err = strconv.Atoi(pityStr)
			if err != nil {
				err = fmt.Errorf("pity factor is not a number")
				return
			}
			if advancedOption.PityFactor < 0 || advancedOption.PityFactor > 100 {
				err = fmt.Errorf("pity factor is not in the correct range")
				return
			}
		}

		advancedOption.MinimumFailure = 3
		minFailStr := req.URL.Query().Get("minFail")
		if minFailStr != "" {
			advancedOption.MinimumFailure, err = strconv.Atoi(minFailStr)
			if err != nil {
				err = fmt.Errorf("min_fail is not a number")
				return
			}
			if advancedOption.MinimumFailure < 0 {
				err = fmt.Errorf("min_fail is not in the correct range")
				return
			}
		}

		advancedOption.IgnoreMissing = false
		ignoreMissingStr := req.URL.Query().Get("ignoreMissing")
		if ignoreMissingStr != "" {
			advancedOption.IgnoreMissing, err = strconv.ParseBool(ignoreMissingStr)
			if err != nil {
				err = errors.WithMessage(err, "expected boolean for ignore missing")
				return
			}
		}

		advancedOption.IgnoreDisruption = true
		ignoreDisruptionsStr := req.URL.Query().Get("ignoreDisruption")
		if ignoreMissingStr != "" {
			advancedOption.IgnoreDisruption, err = strconv.ParseBool(ignoreDisruptionsStr)
			if err != nil {
				err = errors.WithMessage(err, "expected boolean for ignore disruption")
				return
			}
		}

		// TODO: if specified, allow these to override view defaults for start/end time.
		// will need to relocate this outside this else.
		timeStr := req.URL.Query().Get("baseStartTime")
		baseRelease.Start, err = util.ParseCRReleaseTime(baseRelease.Release, timeStr, true, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("base start time in wrong format")
			return
		}
		timeStr = req.URL.Query().Get("baseEndTime")
		baseRelease.End, err = util.ParseCRReleaseTime(baseRelease.Release, timeStr, false, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("base end time in wrong format")
			return
		}
		timeStr = req.URL.Query().Get("sampleStartTime")
		sampleRelease.Start, err = util.ParseCRReleaseTime(sampleRelease.Release, timeStr, true, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("sample start time in wrong format")
			return
		}
		timeStr = req.URL.Query().Get("sampleEndTime")
		sampleRelease.End, err = util.ParseCRReleaseTime(sampleRelease.Release, timeStr, false, crTimeRoundingFactor)
		if err != nil {
			err = fmt.Errorf("sample end time in wrong format")
			return
		}

	}

	// Params below this point can be used with and without views:

	testIDOption.Component = req.URL.Query().Get("component")
	testIDOption.Capability = req.URL.Query().Get("capability")
	testIDOption.TestID = req.URL.Query().Get("testId")

	forceRefreshStr := req.URL.Query().Get("forceRefresh")
	if forceRefreshStr != "" {
		cacheOption.ForceRefresh, err = strconv.ParseBool(forceRefreshStr)
		if err != nil {
			err = errors.WithMessage(err, "expected boolean for force refresh")
			return
		}
	}
	cacheOption.CRTimeRoundingFactor = crTimeRoundingFactor

	return
}

func parseVariantOptions(req *http.Request, allJobVariants apitype.JobVariants) (apitype.ComponentReportRequestVariantOptions, error) {
	var err error
	variantOption := apitype.ComponentReportRequestVariantOptions{}
	columnGroupBy := req.URL.Query().Get("columnGroupBy")
	variantOption.ColumnGroupBy, err = api.VariantsStringToSet(allJobVariants, columnGroupBy)
	if err != nil {
		return variantOption, err
	}
	dbGroupBy := req.URL.Query().Get("dbGroupBy")
	variantOption.DBGroupBy, err = api.VariantsStringToSet(allJobVariants, dbGroupBy)
	if err != nil {
		return variantOption, err
	}
	variantOption.RequestedVariants = map[string]string{}
	// Only the dbGroupBy variants can be specifically requested
	for _, variant := range variantOption.DBGroupBy.List() {
		if value := req.URL.Query().Get(variant); value != "" {
			variantOption.RequestedVariants[variant] = value
		}
	}
	includeVariants := req.URL.Query()["includeVariant"]
	variantOption.IncludeVariants, err = api.IncludeVariantsToMap(allJobVariants, includeVariants)
	return variantOption, err
}

func anyParamSpecified(req *http.Request, paramName ...string) error {
	found := []string{}
	for _, p := range paramName {
		if req.URL.Query().Get(p) != "" {
			found = append(found, p)
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("params cannot be combined with view: %v", found)
	}
	return nil
}
