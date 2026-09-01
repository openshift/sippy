package verify

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/civil"
	"k8s.io/apimachinery/pkg/util/sets"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/releaseoverride"
)

type Runner struct {
	PostgreSQL                  PostgreSQLReader
	BigQuery                    BigQueryReader
	BigQueryInitializationError error
	Config                      *v1config.SippyConfig
	SyntheticReleaseOverrides   *releaseoverride.SyntheticReleaseOverrides
}

func (r *Runner) Run(ctx context.Context, options Options) Result {
	result := Result{}
	releases, err := r.PostgreSQL.Releases(ctx)
	if err != nil {
		for _, check := range options.Checks {
			result.Summaries = append(result.Summaries, operationalSummary(check, options.Release, options.Date, err))
		}
		result.Sort()
		return result
	}
	releases = normalizeReleases(releases)
	if options.Release != "" {
		if !containsRelease(releases, options.Release) {
			err := fmt.Errorf("release %q was not found in release definitions or historical Prow jobs", options.Release)
			for _, check := range options.Checks {
				result.Summaries = append(result.Summaries, operationalSummary(check, options.Release, options.Date, err))
			}
			result.Sort()
			return result
		}
		releases = []string{options.Release}
	}
	if len(releases) == 0 {
		for _, check := range options.Checks {
			result.Summaries = append(result.Summaries, operationalSummary(check, options.Release, options.Date, fmt.Errorf("no releases found")))
		}
		result.Sort()
		return result
	}

	scope := Scope{Date: options.Date, Releases: releases}
	for _, check := range options.Checks {
		verifier := r.verifier(check)
		if verifier == nil {
			err := fmt.Errorf("unsupported verification check %q", check)
			for _, release := range releases {
				result.Summaries = append(result.Summaries, operationalSummary(check, release, options.Date, err))
			}
			continue
		}
		checkResult := verifier.Verify(ctx, scope)
		result.Summaries = append(result.Summaries, checkResult.Summaries...)
		result.Discrepancies = append(result.Discrepancies, checkResult.Discrepancies...)
	}
	result.Sort()
	return result
}

func (r *Runner) verifier(check Check) Verifier {
	switch check {
	case CheckBQCompleteness:
		return &BQCompletenessVerifier{
			PostgreSQL:                  r.PostgreSQL,
			BigQuery:                    r.BigQuery,
			BigQueryInitializationError: r.BigQueryInitializationError,
			Config:                      r.Config,
			SyntheticReleaseOverrides:   r.SyntheticReleaseOverrides,
		}
	case CheckDailyTotals:
		return &DailyTotalsVerifier{PostgreSQL: r.PostgreSQL}
	case CheckCumulativeSummaries:
		return &CumulativeSummariesVerifier{PostgreSQL: r.PostgreSQL}
	default:
		return nil
	}
}

func operationalSummary(check Check, release string, date civil.Date, err error) Summary {
	return Summary{Check: check, Release: release, Date: date, Passed: false, Error: err.Error()}
}

func normalizeReleases(releases []string) []string {
	set := sets.New[string]()
	for _, release := range releases {
		if release = strings.TrimSpace(release); release != "" {
			set.Insert(release)
		}
	}
	return sets.List(set)
}

func containsRelease(releases []string, wanted string) bool {
	return sets.New[string](releases...).Has(wanted)
}
