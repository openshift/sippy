package resolvedissues

import (
	"fmt"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/util/sets"
)

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
	URL       string
	StartTime time.Time
}

// ResolvedIssuesFor returns the resolved issues for the test in the timeframe. These contain the jobruns that were impacted
// Additionally, it returns the number of job runs in the window to suppress.
func ResolvedIssuesFor(releaseString string, variant api.ComponentReportColumnIdentification, testID string, startTime, endTime time.Time) ([]ResolvedIssue, int) {
	registryForRelease := registry.resolvedIssuesFor(release(releaseString))
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
	releaseToResolvedIssues map[release]*resolvedIssueForRelease
}

var registry = newResolvedIssueRegistry()

func newResolvedIssueRegistry() *resolvedIssueRegistry {
	return &resolvedIssueRegistry{
		releaseToResolvedIssues: map[release]*resolvedIssueForRelease{},
	}
}

type resolvedIssueForRelease struct {
	release        release
	resolvedIssues map[resolvedIssueKey][]ResolvedIssue
}

type release string

var (
	release415 release = "4.15"
	release416 release = "4.16"
)

type resolvedIssueKey struct {
	testID  string
	variant api.ComponentReportColumnIdentification
}

func (r *resolvedIssueRegistry) resolvedIssuesFor(release release) *resolvedIssueForRelease {
	return r.releaseToResolvedIssues[release]
}

func newResolvedIssueForRelease(release release) *resolvedIssueForRelease {
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
		testID: testID,
		variant: api.ComponentReportColumnIdentification{
			Network:  variant.Network,
			Upgrade:  variant.Upgrade,
			Arch:     variant.Arch,
			Platform: variant.Platform,
		},
	}
}

func mustAddResolvedIssue(release release, in ResolvedIssue) {
	if err := addResolvedIssue(release, in); err != nil {
		panic(err)
	}
}

func addResolvedIssue(release release, in ResolvedIssue) error {
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
