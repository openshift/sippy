package resolvedissues

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/openshift/sippy/pkg/util/sets"

	"cloud.google.com/go/bigquery"

	"github.com/openshift/sippy/pkg/apis/api"
)

// sync with https://github.com/openshift/sippy/pull/1531/files#diff-3f72919066e1ec3ae4b037dfc91c09ef6d6eac0488762ef35c5a116f73ff1637R237 eventually
const variantArchitecture = "Architecture"
const variantNetwork = "Network"
const variantPlatform = "Platform"
const variantUpgrade = "Upgrade"
const variantVariant = "Variant"

var triageMatchVariants = buildTriageMatchVariants([]string{variantArchitecture, variantNetwork, variantPlatform, variantUpgrade, variantVariant})

func buildTriageMatchVariants(in []string) sets.String {
	if in == nil || len(in) < 1 {
		return nil
	}

	set := sets.NewString()

	for _, l := range in {
		set.Insert(l)
	}

	return set
}
func TransformVariant(variant api.ComponentReportColumnIdentification) []TriagedVariant {

	return []TriagedVariant{{
		Key:   variantArchitecture,
		Value: variant.Arch,
	}, {
		Key:   variantNetwork,
		Value: variant.Network,
	}, {
		Key:   variantPlatform,
		Value: variant.Platform,
	}, {
		Key:   variantUpgrade,
		Value: variant.Upgrade,
	}, {
		Key:   variantVariant,
		Value: variant.Variant,
	}}
}
func KeyForTriagedIssue(testID string, variants []TriagedVariant) TriagedIssueKey {

	matchVariants := make([]TriagedVariant, 0)
	for _, v := range variants {
		// currently we ignore variants that aren't in api.ComponentReportColumnIdentification
		if triageMatchVariants.Has(v.Key) {
			matchVariants = append(matchVariants, v)
		}
	}

	sort.Slice(matchVariants,
		func(a, b int) bool {
			return matchVariants[a].Key < matchVariants[b].Key
		})

	vKey := ""
	for _, v := range matchVariants {
		if len(vKey) > 0 {
			vKey += ","
		}
		vKey += fmt.Sprintf("%s_%s", v.Key, v.Value)
	}

	return TriagedIssueKey{
		testID:   testID,
		variants: vKey,
	}
}

type TriagedIssueKey struct {
	testID   string
	variants string
}

type TriagedIncidentsForRelease struct {
	Release          Release                               `json:"release"`
	TriagedIncidents map[TriagedIssueKey][]TriagedIncident `json:"triaged_incidents"`
}

func NewTriagedIncidentsForRelease(release Release) TriagedIncidentsForRelease {
	return TriagedIncidentsForRelease{
		Release:          release,
		TriagedIncidents: map[TriagedIssueKey][]TriagedIncident{},
	}
}

type TriagedIncidentIssue struct {
	Type           string                 `bigquery:"type"`
	Description    bigquery.NullString    `bigquery:"description"`
	URL            bigquery.NullString    `bigquery:"url"`
	StartDate      time.Time              `bigquery:"start_date"`
	ResolutionDate bigquery.NullTimestamp `bigquery:"resolution_date"`
}

type TriagedIncidentAttribution struct {
	ID         string    `bigquery:"id"`
	UpdateTime time.Time `bigquery:"update_time"`
}

type TriagedVariant struct {
	Key   string `bigquery:"key"`
	Value string `bigquery:"value"`
}

type TriagedIncident struct {
	Release      string                       `bigquery:"release"`
	TestID       string                       `bigquery:"test_id"`
	TestName     string                       `bigquery:"test_name"`
	IncidentID   string                       `bigquery:"incident_id"`
	ModifiedTime time.Time                    `bigquery:"modified_time"`
	Variants     []TriagedVariant             `bigquery:"variants"`
	Issue        TriagedIncidentIssue         `bigquery:"issue"`
	JobRuns      []JobRun                     `bigquery:"job_runs"`
	Attributions []TriagedIncidentAttribution `bigquery:"attributions"`
}

type ResolvedIssue struct {
	TestID   string
	TestName string
	Variant  api.ComponentReportColumnIdentification

	Issue Issue

	ImpactedJobRuns []JobRun
}

type Issue struct {
	IssueType IssueType

	Infrastructure *InfrastructureIssue
	PayloadBug     *PayloadIssue
}

type InfrastructureIssue struct {
	// required
	Description string
	// optional
	JiraURL string
	// TODO consider whether jira URL should be required and get the resolution date from there
	ResolutionDate time.Time
}

type PayloadIssue struct {
	// required
	PullRequestURL string

	// TODO switch to detecting this from the payload
	// required
	ResolutionDate time.Time
}

type IssueType string

var (
	Infrastructure IssueType = "Infrastructure"
	PayloadBug     IssueType = "PayloadBug"
)

type JobRun struct {
	URL       string    `bigquery:"url"`
	StartTime time.Time `bigquery:"start_time"`
}

// ResolvedIssuesFor returns the resolved issues for the test in the timeframe. These contain the jobruns that were impacted
// Additionally, it returns the number of job runs in the window to suppress.
func ResolvedIssuesFor(releaseString string, variant api.ComponentReportColumnIdentification, testID string, startTime, endTime time.Time) ([]ResolvedIssue, int) {
	registryForRelease := registry.resolvedIssuesFor(Release(releaseString))
	if registryForRelease == nil {
		return nil, 0
	}
	inKey := keyFor(testID, variant)

	resolvedIssues := registryForRelease.resolvedIssuesWithinRange(inKey, startTime, endTime)
	impactedJobRuns := sets.NewString() // because multiple issues could impact the same job run, be sure to count each job run only once
	numJobRunsToSuppress := 0
	for _, resolvedIssue := range resolvedIssues {
		for _, impactedJobRun := range resolvedIssue.ImpactedJobRuns {
			if impactedJobRuns.Has(impactedJobRun.URL) {
				continue
			}
			impactedJobRuns.Insert(impactedJobRun.URL)

			if impactedJobRun.StartTime.After(startTime) && impactedJobRun.StartTime.Before(endTime) {
				numJobRunsToSuppress++
			}
		}
	}

	return resolvedIssues, numJobRunsToSuppress
}

type resolvedIssueRegistry struct {
	releaseToResolvedIssues map[Release]*resolvedIssueForRelease
}

var registry = newResolvedIssueRegistry()

func newResolvedIssueRegistry() *resolvedIssueRegistry {
	return &resolvedIssueRegistry{
		releaseToResolvedIssues: map[Release]*resolvedIssueForRelease{},
	}
}

type resolvedIssueForRelease struct {
	release        Release
	resolvedIssues map[resolvedIssueKey][]ResolvedIssue
}

type Release string

var (
	release415 Release = "4.15"
	release416 Release = "4.16"
)

type resolvedIssueKey struct {
	TestID  string                                  `json:"test_id"`
	Variant api.ComponentReportColumnIdentification `json:"variant"`
}

// implement encoding.TextMarshaler for json map key marshalling support
func (s resolvedIssueKey) MarshalText() (text []byte, err error) {
	type t resolvedIssueKey
	return json.Marshal(t(s))
}

func (s *resolvedIssueKey) UnmarshalText(text []byte) error {
	type t resolvedIssueKey
	return json.Unmarshal(text, (*t)(s))
}

func (r *resolvedIssueRegistry) resolvedIssuesFor(release Release) *resolvedIssueForRelease {
	return r.releaseToResolvedIssues[release]
}

func newResolvedIssueForRelease(release Release) *resolvedIssueForRelease {
	return &resolvedIssueForRelease{
		release:        release,
		resolvedIssues: map[resolvedIssueKey][]ResolvedIssue{},
	}
}

func (r *resolvedIssueForRelease) resolvedIssuesWithinRange(key resolvedIssueKey, startTime, endTime time.Time) []ResolvedIssue {
	allResolvedIssues := r.resolvedIssues[key]
	allRelevantIssues := []ResolvedIssue{}
	for i, resolvedIssue := range allResolvedIssues {
		for _, impactedJobRun := range resolvedIssue.ImpactedJobRuns {
			if impactedJobRun.StartTime.After(startTime) && impactedJobRun.StartTime.Before(endTime) {
				allRelevantIssues = append(allRelevantIssues, allResolvedIssues[i])
				break
			}
		}
	}
	return allRelevantIssues
}

func (r *resolvedIssueForRelease) addResolvedIssue(in ResolvedIssue) error {
	if len(in.TestID) == 0 {
		return fmt.Errorf("testID must be specified")
	}
	if len(in.TestName) == 0 {
		return fmt.Errorf("testName must be specified")
	}
	if len(in.Variant.Network) == 0 {
		return fmt.Errorf("network must be specified")
	}
	if len(in.Variant.Arch) == 0 {
		return fmt.Errorf("arch must be specified")
	}
	if len(in.Variant.Platform) == 0 {
		return fmt.Errorf("platform must be specified")
	}
	if len(in.Variant.Upgrade) == 0 {
		return fmt.Errorf("upgrade must be specified")
	}
	switch in.Issue.IssueType {
	case Infrastructure:
		if in.Issue.Infrastructure == nil {
			return fmt.Errorf("infrastructure must be specified")
		}
		if in.Issue.PayloadBug != nil {
			return fmt.Errorf("payloadBug must not be specified")
		}
		if in.Issue.Infrastructure.ResolutionDate.IsZero() {
			return fmt.Errorf("resolutionDate must be specified")
		}
	case PayloadBug:
		if in.Issue.Infrastructure != nil {
			return fmt.Errorf("infrastructure must not be specified")
		}
		if in.Issue.PayloadBug == nil {
			return fmt.Errorf("payloadBug must be specified")
		}
		if in.Issue.PayloadBug.ResolutionDate.IsZero() {
			return fmt.Errorf("resolutionDate must be specified")
		}

	default:
		return fmt.Errorf("unrecognized issue type")
	}
	if len(in.ImpactedJobRuns) == 0 {
		return fmt.Errorf("impactedJobRuns must be specified")
	}
	for i, jobRun := range in.ImpactedJobRuns {
		if len(jobRun.URL) == 0 {
			return fmt.Errorf("impactedJobRuns[%d] must have url", i)
		}
		if jobRun.StartTime.IsZero() {
			return fmt.Errorf("impactedJobRuns[%d] must have startTime", i)
		}
	}

	inKey := keyFor(in.TestID, in.Variant)
	r.resolvedIssues[inKey] = append(r.resolvedIssues[inKey], in)

	return nil
}

func keyFor(testID string, variant api.ComponentReportColumnIdentification) resolvedIssueKey {
	return resolvedIssueKey{
		TestID: testID,
		Variant: api.ComponentReportColumnIdentification{
			Network:  variant.Network,
			Upgrade:  variant.Upgrade,
			Arch:     variant.Arch,
			Platform: variant.Platform,
		},
	}
}

func mustAddResolvedIssue(release Release, in ResolvedIssue) {
	if err := addResolvedIssue(release, in); err != nil {
		panic(err)
	}
}

func addResolvedIssue(release Release, in ResolvedIssue) error {
	if len(release) == 0 {
		return fmt.Errorf("release must be specified")
	}
	targetMap := registry.resolvedIssuesFor(release)
	if targetMap == nil {
		targetMap = newResolvedIssueForRelease(release)
		registry.releaseToResolvedIssues[release] = targetMap
	}

	return targetMap.addResolvedIssue(in)
}

func mustTime(in string) time.Time {
	out, err := time.Parse(time.RFC3339, in)
	if err != nil {
		panic(err)
	}
	return out
}
