package resolvedissues

import (
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

var triageMatchVariants = buildTriageMatchVariants([]string{variantArchitecture, variantNetwork, variantPlatform, variantUpgrade})

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
	triagedVariants := []TriagedVariant{}
	for name, value := range variant.Variants {
		// For now, we only use the defined match variants
		if triageMatchVariants.Has(name) {
			triagedVariants = append(triagedVariants, TriagedVariant{Key: name, Value: value})
		}
	}
	return triagedVariants
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

type Release string

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
