package jiraintegrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db/query"
	log "github.com/sirupsen/logrus"
	"github.com/trivago/tgo/tcontainer"
	jirautil "sigs.k8s.io/prow/pkg/jira"
)

const (
	jiraProjectKey                             = "OCPBUGS"
	jiraCreator                                = "'kenzhang@redhat.com'"
	jiraLabelIngregration                      = "Regression"
	jiraCustomFieldReleaseBlockerName          = "customfield_12319743"
	jiraCustomFieldReleaseBlockerValueApproved = "Approved"

	jiraStatusNew        = "New"
	jiraStatusInProgress = "In Progress"
	jiraStatusAssigned   = "Assigned"
	jiraStatusModified   = "Modified"
	jiraStatusQA         = "QA"
	jiraStatusVerified   = "Verified"
	jiraStatusClosed     = "Closed"

	releaseStatusDevelopment = "Development"

	// fixCheckMinimumDays defines the minimum days (since fix date) we need to wait to verify a fix.
	// Typically, it takes multiple days to have sufficient data available to make this determination.
	fixCheckMinimumDays = 3 * time.Hour * 24
)

type JiraIntegrator struct {
	jiraClient   jirautil.Client
	bqClient     *bqclient.Client
	prowURL      string
	gcsBucket    string
	cacheOptions cache.RequestOptions
	views        []crtype.View
	releases     []query.Release
	sippyURL     string
}

func NewJiraIntegrator(jiraClient jirautil.Client, bqClient *bqclient.Client, prowURL, gcsBucket string,
	cacheOptions cache.RequestOptions, views []crtype.View, releases []query.Release, sippyURL string) (JiraIntegrator, error) {
	j := JiraIntegrator{
		jiraClient:   jiraClient,
		bqClient:     bqClient,
		prowURL:      prowURL,
		gcsBucket:    gcsBucket,
		cacheOptions: cacheOptions,
		releases:     releases,
		sippyURL:     sippyURL,
	}
	if bqClient == nil || bqClient.BQ == nil {
		return j, fmt.Errorf("we don't have a bigquery client for jira integrator")
	}

	if bqClient.Cache == nil {
		return j, fmt.Errorf("we don't have a cache configured for jira integrator")
	}

	for _, v := range views {
		if v.JiraIntegration.Enabled {
			j.views = append(j.views, v)
		}
	}
	return j, nil
}

func (j JiraIntegrator) getRequestOptionForView(view crtype.View) (crtype.RequestOptions, error) {
	baseRelease, err := componentreadiness.GetViewReleaseOptions("basis", view.BaseRelease, j.cacheOptions.CRTimeRoundingFactor)
	if err != nil {
		return crtype.RequestOptions{}, err
	}

	sampleRelease, err := componentreadiness.GetViewReleaseOptions("sample", view.SampleRelease, j.cacheOptions.CRTimeRoundingFactor)
	if err != nil {
		return crtype.RequestOptions{}, err
	}

	variantOption := view.VariantOptions
	advancedOption := view.AdvancedOptions

	// Get component readiness report
	reportOpts := crtype.RequestOptions{
		BaseRelease:    baseRelease,
		SampleRelease:  sampleRelease,
		TestIDOption:   view.TestIDOption,
		VariantOption:  variantOption,
		AdvancedOption: advancedOption,
		CacheOption:    j.cacheOptions,
	}
	return reportOpts, nil
}

func (j JiraIntegrator) getComponentReportForView(view crtype.View) (crtype.ComponentReport, error) {
	reportOpts, err := j.getRequestOptionForView(view)
	if err != nil {
		return crtype.ComponentReport{}, fmt.Errorf("failed to get request option for view %s with error %v", view.Name, err)
	}

	report, errs := componentreadiness.GetComponentReportFromBigQuery(j.bqClient, j.prowURL, j.gcsBucket, reportOpts)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return crtype.ComponentReport{}, fmt.Errorf("component report generation encountered errors: " + strings.Join(strErrors, "; "))
	}
	return report, nil
}

func getProbabilityString(status crtype.Status, fisherExact float64) string {
	if status == crtype.SignificantRegression || status == crtype.ExtremeRegression {
		return fmt.Sprintf("Probability of significant regression: %.2f%%\n\n", (1-fisherExact)*100)
	} else if status == crtype.SignificantImprovement {
		return fmt.Sprintf("Probability of significant improvement: %.2f%%\n\n", (1-fisherExact)*100)
	}
	return "There is no significant evidence of regression\n\n"
}

func getStatsString(prefix string, stats crtype.TestDetailsReleaseStats, from, end string) string {
	return fmt.Sprintf(prefix+" Release: %s\n"+
		"\tStart Time: %s\n"+
		"\tEnd Time: %s\n"+
		"\tSuccess Rate: %.2f%%\n"+
		"\tSuccesses: %d\n"+
		"\tFailures: %d\n"+
		"\tFlakes: %d\n\n",
		stats.Release, from, end, stats.SuccessRate*100, stats.SuccessCount, stats.FailureCount, stats.FlakeCount,
	)
}

// getExistingIssuesForComponent gets existing issues for a component based on
// (a) have Regression label defined by jiraLabelIngregration
// (b) has the "Affects Version/s" set to the sample version,
// (c) were reported by the CR JIRA service account
// Issues will be ordered by creation time
func (j JiraIntegrator) getExistingIssuesForComponent(view crtype.View, component string) ([]jira.Issue, error) {
	searchOptions := jira.SearchOptions{
		MaxResults: 1,
		Fields: []string{
			"key",
			"status",
			"resolutiondate",
			jiraCustomFieldReleaseBlockerName,
			"unknowns",
		},
	}
	jqlQuery := fmt.Sprintf("project=%s&&component='%s'&&creator=%s&&affectedVersion=%s&&labels in (%s) ORDER BY createdDate",
		jiraProjectKey, component, jiraCreator, view.SampleRelease.Release, jiraLabelIngregration)
	issues, _, err := j.jiraClient.SearchWithContext(context.Background(), jqlQuery, &searchOptions)
	return issues, err
}

func (j JiraIntegrator) isPreRelease(release string) bool {
	for _, r := range j.releases {
		if r.Release == release {
			if r.Status == releaseStatusDevelopment {
				return true
			}
			break
		}
	}
	return false
}

func isReleaseBlockerApproved(existing *jira.Issue) bool {
	type releaseBlockerField struct {
		Self     string `json:"self"`
		ID       string `json:"id"`
		Disabled bool   `json:"disabled"`
		Value    string `json:"value"`
	}
	var obj *releaseBlockerField
	err := jirautil.GetUnknownField(jiraCustomFieldReleaseBlockerName, existing, func() interface{} {
		obj = &releaseBlockerField{}
		return obj
	})
	if err == nil && obj.Value == jiraCustomFieldReleaseBlockerValueApproved {
		return true
	}
	return false
}

// updateExistingJiraIssue updates existing issue by
// a. adding a new comment containing a CR link with the most recently analyzed time window where the regression is still manifesting.
// b. if pre-release, label the ticket as a Release Blocker if someone removed it
func (j JiraIntegrator) updateExistingJiraIssue(view crtype.View, existing *jira.Issue) error {
	absUrl, _, err := j.getComponentReadinessURLsForView(view)
	if err != nil {
		return err
	}
	comment := fmt.Sprintf(`This bug is still seen in component readiness. Here is [the current link|%s] for your convenience`, absUrl)

	_, err = j.jiraClient.AddComment(existing.ID, &jira.Comment{Body: comment})
	if err != nil {
		return err
	}

	// Set Release Blocker
	if !isReleaseBlockerApproved(existing) {
		_, err = j.updateReleaseBlocker(existing, view.SampleRelease.Release)
		if err != nil {
			return err
		}
	}

	return nil
}

// getComponentReadinessURLsForView generates two URL, one with absolute timing params at this moment this is called
// and one with view.
func (j JiraIntegrator) getComponentReadinessURLsForView(view crtype.View) (string, string, error) {
	reportOpts, err := j.getRequestOptionForView(view)
	if err != nil {
		return "", "", err
	}
	absURL := j.sippyURL + "/sippy-ng/component_readiness/"
	// Create a URL values object
	values := url.Values{}
	if reportOpts.TestIDOption.TestID != "" {
		absURL += "env_test?"
		values.Add("testId", reportOpts.TestIDOption.TestID)
		values.Add("capability", reportOpts.TestIDOption.Capability)
		values.Add("component", reportOpts.TestIDOption.Component)
	} else if reportOpts.TestIDOption.Capability != "" {
		absURL += "env_capability?"
		values.Add("capability", reportOpts.TestIDOption.Capability)
		values.Add("component", reportOpts.TestIDOption.Component)
	} else if reportOpts.TestIDOption.Component != "" {
		absURL += "env_capabilities?"
		values.Add("component", reportOpts.TestIDOption.Component)
	} else {
		absURL += "main?"
	}

	if reportOpts.BaseRelease.Release != "" {
		values.Add("baseRelease", reportOpts.BaseRelease.Release)
		values.Add("baseStartTime", reportOpts.BaseRelease.Start.UTC().Format(time.RFC3339))
		values.Add("baseEndTime", reportOpts.BaseRelease.End.UTC().Format(time.RFC3339))
	}
	if reportOpts.SampleRelease.Release != "" {
		values.Add("sampleRelease", reportOpts.SampleRelease.Release)
		values.Add("sampleStartTime", reportOpts.SampleRelease.Start.UTC().Format(time.RFC3339))
		values.Add("sampleEndTime", reportOpts.SampleRelease.End.UTC().Format(time.RFC3339))
	}
	values.Add("columnGroupBy", strings.Join(reportOpts.VariantOption.ColumnGroupBy.List(), ","))
	for name, variants := range reportOpts.VariantOption.IncludeVariants {
		for _, v := range variants {
			values.Add("includeVariant", fmt.Sprintf("%s:%s", name, v))
		}
	}

	values.Add("confidence", fmt.Sprintf("%d", reportOpts.AdvancedOption.Confidence))
	if reportOpts.AdvancedOption.IgnoreDisruption {
		values.Add("ignoreDisruption", "1")
	} else {
		values.Add("ignoreDisruption", "0")
	}
	if reportOpts.AdvancedOption.IgnoreMissing {
		values.Add("ignoreMissing", "1")
	} else {
		values.Add("ignoreMissing", "0")
	}
	values.Add("minFail", fmt.Sprintf("%d", reportOpts.AdvancedOption.MinimumFailure))
	values.Add("pity", fmt.Sprintf("%d", reportOpts.AdvancedOption.PityFactor))

	absURL += values.Encode()
	viewURL := j.sippyURL + "/sippy-ng/component_readiness/main?view=" + view.Name
	return absURL, viewURL, nil
}

// createNewJiraIssueForRegressions creates new issue for components by
// a. setting the ticket's Affects Version/s= sample version.
// b  adding the Regression label defined by jiraLabelIngregration.
// c. setting a description with links to CR
// d. for pre-release, setting "Release Blocker" label to Approved
func (j JiraIntegrator) createNewJiraIssueForRegressions(view crtype.View, component string, tests []crtype.ReportTestSummary, linkedIssue *jira.Issue) (*jira.Issue, error) {
	if len(tests) > 0 {
		description := `Component Readiness has found a potential regression in the following tests:`
		for i, test := range tests {
			// Only show stats for the worst regression
			if i == 0 {
				description += fmt.Sprintf("\n h4. Most Regressed Test:\n{code}%s{code}\n", test.TestName)
				description += getProbabilityString(test.ReportStatus, test.FisherExact)
				description += getStatsString("Sample (being evaluated)", test.SampleStats, view.SampleRelease.RelativeStart, view.SampleRelease.RelativeEnd)
				description += getStatsString("Base (historical)", test.BaseStats, view.BaseRelease.RelativeStart, view.BaseRelease.RelativeEnd)
				if len(tests) > 1 {
					description += fmt.Sprintf("\n h4. Other Regressed Tests:\n")
					description += fmt.Sprintf("\nThe following tests are also regressed in the same component readiness report. They might not be related to the most regressed test above. We only create one issue per component and therefore group them here. Feel free to create new issues if they are unrelated.\n")
				}
			} else {
				description += fmt.Sprintf("{code}%s{code}\n", test.TestName)
				description += getProbabilityString(test.ReportStatus, test.FisherExact)
			}
		}
		if linkedIssue != nil {
			description += fmt.Sprintf("\n h4. Potentially Related Issues:\n")
			description += fmt.Sprintf("\n* This regression might be related to [%s|%s]. Feel free to link it if found related.\n", linkedIssue.Key, linkedIssue.Self)
		}

		absUrl, viewUrl, err := j.getComponentReadinessURLsForView(view)
		if err != nil {
			return nil, err
		}
		description += fmt.Sprintf("\n h4. Useful Component Readiness Links:\n")
		description += fmt.Sprintf("\nWe are proving the following two links for your convenience:\n")
		description += fmt.Sprintf("\n- Click [here|%s] to access the component readiness page generated at the time this issue was created.\n", absUrl)
		description += fmt.Sprintf("\n- Click [here|%s] to access the component readiness page that will be generated at the time when it is clicked. This is useful for developers to verify their fixes.\n", viewUrl)

		summary := fmt.Sprintf("Regression with test %s", tests[0].TestName)
		issue := jira.Issue{
			Fields: &jira.IssueFields{
				Description: description,
				Type: jira.IssueType{
					Name: "Bug",
				},
				Project: jira.Project{
					Key: jiraProjectKey,
				},
				Components: []*jira.Component{
					{
						Name: component,
					},
				},
				Summary: summary,
				AffectsVersions: []*jira.AffectsVersion{
					{
						Name: view.SampleRelease.Release,
					},
				},
				Labels: []string{jiraLabelIngregration},
			},
		}
		created, err := j.jiraClient.CreateIssue(&issue)
		if err != nil {
			return created, err
		}
		// Set Release Blocker field. Jira does not allow setting those during creation. So do it in separate step.
		return j.updateReleaseBlocker(created, view.SampleRelease.Release)
	}
	return nil, nil
}

func (j JiraIntegrator) updateReleaseBlocker(issue *jira.Issue, release string) (*jira.Issue, error) {
	if j.isPreRelease(release) {
		unknowns := tcontainer.NewMarshalMap()
		unknowns[jiraCustomFieldReleaseBlockerName] = map[string]string{"value": jiraCustomFieldReleaseBlockerValueApproved}
		issue := jira.Issue{
			Key: issue.Key,
			Fields: &jira.IssueFields{
				Unknowns: unknowns,
			},
		}
		return j.jiraClient.UpdateIssue(&issue)
	}
	return issue, nil
}

func (j JiraIntegrator) getComponentRegressedTestsFromReport(report crtype.ComponentReport) map[string][]crtype.ReportTestSummary {
	componentRegressedTests := map[string][]crtype.ReportTestSummary{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, test := range col.RegressedTests {
				componentRegressedTests[test.Component] = append(componentRegressedTests[test.Component], test)
			}
		}
	}
	for component, tests := range componentRegressedTests {
		sort.Slice(tests, func(i, j int) bool {
			return tests[i].ReportStatus < tests[j].ReportStatus
		})
		componentRegressedTests[component] = tests
	}
	return componentRegressedTests
}

func (j JiraIntegrator) integrateJiraForView(view crtype.View) error {

	logger := log.WithField("view", view.Name)
	logger.Info("jira integration for view")

	report, err := j.getComponentReportForView(view)
	if err != nil {
		logger.WithError(err).Error("error getting report for view")
	}

	componentRegressedTests := j.getComponentRegressedTestsFromReport(report)
	for component, tests := range componentRegressedTests {
		// fetch jira bugs
		// resetting this to our component to test
		component = "Test Framework"
		issues, err := j.getExistingIssuesForComponent(view, component)
		if err != nil {
			log.WithError(err).Error("error getting existing jira issues")
		}

		// No existing issues, create new one
		if len(issues) == 0 {
			_, err := j.createNewJiraIssueForRegressions(view, component, tests, nil)
			if err != nil {
				log.WithError(err).Error("error creating jira issue")
			}
		} else {
			selected := issues[0]
			switch selected.Fields.Status.Name {
			case jiraStatusNew, jiraStatusInProgress, jiraStatusAssigned, jiraStatusModified:
				// New/Assigned/In Progress/Modified
				err := j.updateExistingJiraIssue(view, &selected)
				if err != nil {
					log.WithError(err).Error("error updating jira issue with comment")
				}
			case jiraStatusQA, jiraStatusVerified, jiraStatusClosed:
				// QA/Verified/Closed
				resolutionDate := time.Time(selected.Fields.Resolutiondate)
				if view.SampleRelease.Start.After(resolutionDate) {
					// Existing issue does not cover current regression
					_, err := j.createNewJiraIssueForRegressions(view, component, tests, nil)
					if err != nil {
						log.WithError(err).Error("error creating jira issue")
					}
				} else {
					// Overlap between current analysis and jira card fix. Do two more analysis:
					// a. Scope Check: Run with a sample start date of resolutionDate-2 weeks and end resolutionDate.
					//    If current analysis contains new tests not covered by scope check, create new card.
					// b. Fix Check: Run with a sample start date of resolutionDate and the original end date. Only
					//    do this after a reasonable number of days has passed.
					scopeView := view
					scopeView.TestIDOption.Component = component
					scopeView.SampleRelease.RelativeStart = resolutionDate.Add(-14 * time.Hour * 24).Format(time.RFC3339)
					scopeView.SampleRelease.RelativeEnd = resolutionDate.Format(time.RFC3339)
					scopeReport, err := j.getComponentReportForView(scopeView)
					if err != nil {
						logger.WithError(err).Error("error getting report for scope check")
					}

					// Identify tests only appearing in current report, not scope report
					scopeRegressedTests := map[crtype.RowIdentification]map[crtype.ColumnID]crtype.ReportTestSummary{}
					for _, row := range scopeReport.Rows {
						for _, col := range row.Columns {
							for _, test := range col.RegressedTests {
								if _, ok := scopeRegressedTests[test.RowIdentification]; !ok {
									scopeRegressedTests[row.RowIdentification] = map[crtype.ColumnID]crtype.ReportTestSummary{}
								}
								columnKeyBytes, err := json.Marshal(test.ColumnIdentification)
								if err != nil {
									return err
								}
								scopeRegressedTests[test.RowIdentification][crtype.ColumnID(columnKeyBytes)] = test
							}
						}
					}
					newTests := []crtype.ReportTestSummary{}
					for _, test := range tests {
						columnKeyBytes, err := json.Marshal(test.ColumnIdentification)
						if err != nil {
							return err
						}
						_, ok := scopeRegressedTests[test.RowIdentification]
						if !ok {
							newTests = append(newTests, test)
						} else if _, ok := scopeRegressedTests[test.RowIdentification][crtype.ColumnID(columnKeyBytes)]; !ok {
							newTests = append(newTests, test)
						}
					}
					if len(newTests) > 0 {
						// Any tests not covered by scope check is considered new
						_, err := j.createNewJiraIssueForRegressions(view, component, newTests, &selected)
						if err != nil {
							log.WithError(err).Error("error creating jira issue")
						}
					} else {
						// This means scope report contains all tests from current report, verify fix
						if resolutionDate.Add(fixCheckMinimumDays).Before(view.SampleRelease.End) {
							fixView := view
							fixView.TestIDOption.Component = component
							fixView.SampleRelease.RelativeStart = resolutionDate.Format(time.RFC3339)
							fixReport, err := j.getComponentReportForView(fixView)
							if err != nil {
								logger.WithError(err).Error("error getting report for fix check")
							}
							regressedTests := j.getComponentRegressedTestsFromReport(fixReport)
							if tests, ok := regressedTests[component]; ok {
								_, err := j.createNewJiraIssueForRegressions(fixView, component, tests, nil)
								if err != nil {
									log.WithError(err).Error("error creating jira issue")
								}
							}
						}
					}
				}
			}
		}
		// test code. Only process one component for testing purposes
		break
	}
	return nil
}

func (j JiraIntegrator) IntegrateJira() error {
	log.Infof("Start integrating component readiness regressions with Jira")
	for _, view := range j.views {
		err := j.integrateJiraForView(view)
		if err != nil {
			return err
		}
	}
	log.Infof("Done integrating component readiness regressions with Jira")
	return nil
}
