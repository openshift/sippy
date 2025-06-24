package jiraautomator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	log "github.com/sirupsen/logrus"
	"github.com/trivago/tgo/tcontainer"
	"google.golang.org/api/iterator"
	jirautil "sigs.k8s.io/prow/pkg/jira"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	jiratype "github.com/openshift/sippy/pkg/apis/jira/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	releaseStatusDevelopment = "Development"

	// fixCheckWaitPeriod defines the minimum days (since fix date) we need to wait to verify a fix.
	// Typically, it takes multiple days to have sufficient data available to make this determination.
	fixCheckWaitPeriod = 3 * time.Hour * 24
)

type Variant struct {
	Name  string
	Value string
}

type JiraComponent struct {
	Project   string
	Component string
}

type JiraAutomator struct {
	jiraClient   jirautil.Client
	bqClient     *bqclient.Client
	dbc          *db.DB
	cacheOptions cache.RequestOptions
	views        []crview.View
	releases     []v1.Release
	sippyURL     string
	// columnThresholds defines a threshold for the number of red cells in a column.
	// When the number of red cells of a column is over this threshold, a jira card will be created for the
	// Variant (column) based jira component. No other jira cards will be created per component row.
	columnThresholds           map[Variant]int
	includeComponents          sets.String
	jiraAccount                string
	dryRun                     bool
	variantToJiraComponents    map[Variant]JiraComponent
	variantJunitTableOverrides []configv1.VariantJunitTableOverride
}

func NewJiraAutomator(
	jiraClient jirautil.Client,
	bqClient *bqclient.Client,
	dbc *db.DB,
	cacheOptions cache.RequestOptions,
	views []crview.View,
	releases []v1.Release,
	sippyURL, jiraAccount string,
	includeComponents sets.String,
	columnThresholds map[Variant]int,
	dryRun bool,
	variantToJiraComponents map[Variant]JiraComponent,
	variantJunitTableOverrides []configv1.VariantJunitTableOverride,
) (JiraAutomator, error) {

	j := JiraAutomator{
		jiraClient:                 jiraClient,
		bqClient:                   bqClient,
		dbc:                        dbc,
		cacheOptions:               cacheOptions,
		releases:                   releases,
		sippyURL:                   sippyURL,
		columnThresholds:           columnThresholds,
		includeComponents:          includeComponents,
		jiraAccount:                jiraAccount,
		dryRun:                     dryRun,
		variantToJiraComponents:    variantToJiraComponents,
		variantJunitTableOverrides: variantJunitTableOverrides,
	}
	if bqClient == nil || bqClient.BQ == nil {
		return j, fmt.Errorf("we don't have a bigquery client for jira integrator")
	}

	if bqClient.Cache == nil {
		return j, fmt.Errorf("we don't have a cache configured for jira integrator")
	}

	for _, v := range views {
		if v.AutomateJira.Enabled {
			j.views = append(j.views, v)
		}
	}
	return j, nil
}

func (j JiraAutomator) getRequestOptionForView(view crview.View) (reqopts.RequestOptions, error) {
	baseRelease, err := componentreadiness.GetViewReleaseOptions(j.releases, "basis", view.BaseRelease, j.cacheOptions.CRTimeRoundingFactor)
	if err != nil {
		return reqopts.RequestOptions{}, err
	}

	sampleRelease, err := componentreadiness.GetViewReleaseOptions(j.releases, "sample", view.SampleRelease, j.cacheOptions.CRTimeRoundingFactor)
	if err != nil {
		return reqopts.RequestOptions{}, err
	}

	variantOption := view.VariantOptions
	advancedOption := view.AdvancedOptions

	// Get component readiness report
	reportOpts := reqopts.RequestOptions{
		BaseRelease:   baseRelease,
		SampleRelease: sampleRelease,
		TestIDOptions: []reqopts.TestIdentification{
			view.TestIDOption,
		},
		VariantOption:  variantOption,
		AdvancedOption: advancedOption,
		CacheOption:    j.cacheOptions,
	}
	return reportOpts, nil
}

func (j JiraAutomator) getComponentReportForView(view crview.View) (crtype.ComponentReport, error) {
	reportOpts, err := j.getRequestOptionForView(view)
	if err != nil {
		return crtype.ComponentReport{}, fmt.Errorf("failed to get request option for view %s with error %v", view.Name, err)
	}

	// Passing empty gcs bucket and prow URL, they are not needed outside test details reports
	report, errs := componentreadiness.GetComponentReportFromBigQuery(context.Background(), j.bqClient, j.dbc, reportOpts, j.variantJunitTableOverrides)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return crtype.ComponentReport{}, fmt.Errorf("component report generation encountered errors: %s", strings.Join(strErrors, "; "))
	}
	return report, nil
}

// getExistingIssuesForComponent gets existing issues for a component based on
// (a) have Regression label defined by LabelJiraAutomator
// (b) has the "Affects Version/s" set to the sample version,
// (c) were reported by the CR JIRA service account
// Issues will be ordered by creation time
func (j JiraAutomator) getExistingIssuesForComponent(view crview.View, component JiraComponent) ([]jira.Issue, error) {
	searchOptions := jira.SearchOptions{
		MaxResults: 1,
		Fields: []string{
			"key",
			"status",
			"resolutiondate",
			jiratype.CustomFieldReleaseBlockerName,
			"unknowns",
		},
	}
	jqlQuery := fmt.Sprintf("project=%s&&component='%s'&&creator='%s'&&affectedVersion=%s&&labels in (%s) ORDER BY createdDate",
		component.Project, component.Component, j.jiraAccount, view.SampleRelease.Name, jiratype.LabelJiraAutomator)
	issues, _, err := j.jiraClient.SearchWithContext(context.Background(), jqlQuery, &searchOptions)
	return issues, err
}

func (j JiraAutomator) isPreRelease(release string) bool {
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
	err := jirautil.GetUnknownField(jiratype.CustomFieldReleaseBlockerName, existing, func() interface{} {
		obj = &releaseBlockerField{}
		return obj
	})
	if err == nil && obj.Value == jiratype.CustomFieldReleaseBlockerValueApproved {
		return true
	}
	return false
}

// updateExistingJiraIssue updates existing issue by
// a. adding a new comment containing a CR link with the most recently analyzed time window where the regression is still manifesting.
// b. if pre-release, label the ticket as a Release Blocker if someone removed it
func (j JiraAutomator) updateExistingJiraIssue(view crview.View, existing *jira.Issue) error {
	absURL, _, err := j.getComponentReadinessURLsForView(view)
	if err != nil {
		return err
	}
	comment := fmt.Sprintf(`This bug is still seen in component readiness. Here is [the current link|%s] for your convenience`, absURL)

	if j.dryRun {
		fmt.Fprintf(os.Stdout, "\n====================================================================\n")
		fmt.Fprintf(os.Stdout, "\nUpdating issue %s with comment\n%s", existing.ID, comment)
		fmt.Fprintf(os.Stdout, "\n====================================================================\n")
	} else {
		_, err = j.jiraClient.AddComment(existing.ID, &jira.Comment{Body: comment})
		if err != nil {
			return err
		}
	}

	// Set Release Blocker
	if !isReleaseBlockerApproved(existing) {
		return j.updateReleaseBlocker(existing, view.SampleRelease.Name)
	}

	return nil
}

// getComponentReadinessURLsForView generates two URL, one with absolute timing params at this moment this is called
// and one with view.
func (j JiraAutomator) getComponentReadinessURLsForView(view crview.View) (string, string, error) {
	reportOpts, err := j.getRequestOptionForView(view)
	if err != nil {
		return "", "", err
	}
	absURL := j.sippyURL + "/sippy-ng/component_readiness/"
	// Create a URL values object
	values := url.Values{}
	if len(reportOpts.TestIDOptions) > 0 {
		if reportOpts.TestIDOptions[0].TestID != "" {
			absURL += "env_test?"
			values.Add("testId", reportOpts.TestIDOptions[0].TestID)
			values.Add("capability", reportOpts.TestIDOptions[0].Capability)
			values.Add("component", reportOpts.TestIDOptions[0].Component)
		} else if reportOpts.TestIDOptions[0].Capability != "" {
			absURL += "env_capability?"
			values.Add("capability", reportOpts.TestIDOptions[0].Capability)
			values.Add("component", reportOpts.TestIDOptions[0].Component)
		} else if reportOpts.TestIDOptions[0].Component != "" {
			absURL += "env_capabilities?"
			values.Add("component", reportOpts.TestIDOptions[0].Component)
		}
	} else {
		absURL += "main?"
	}

	if reportOpts.BaseRelease.Name != "" {
		values.Add("baseRelease", reportOpts.BaseRelease.Name)
		values.Add("baseStartTime", reportOpts.BaseRelease.Start.UTC().Format(time.RFC3339))
		values.Add("baseEndTime", reportOpts.BaseRelease.End.UTC().Format(time.RFC3339))
	}
	if reportOpts.SampleRelease.Name != "" {
		values.Add("sampleRelease", reportOpts.SampleRelease.Name)
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
// b  adding the Regression label defined by LabelJiraAutomator.
// c. setting a description with links to CR
// d. for pre-release, setting "Release Blocker" label to Approved
func (j JiraAutomator) createNewJiraIssueForRegressions(view crview.View, component JiraComponent, tests []crtype.ReportTestSummary, linkedIssue *jira.Issue) error {
	if len(tests) > 0 {
		description := `Component Readiness has found a potential regression in the following tests:`
		for _, test := range tests {
			description += fmt.Sprintf("{code}%s{code}\n", test.TestName)
			description += strings.Join(test.Explanations, "\n")
		}
		if linkedIssue != nil {
			description += "\n h4. Potentially Related Issues:\n"
			description += fmt.Sprintf("\n* This regression might be related to [%s|%s]. Feel free to link it if found related.\n", linkedIssue.Key, linkedIssue.Self)
		}

		absURL, viewURL, err := j.getComponentReadinessURLsForView(view)
		if err != nil {
			return err
		}
		description += "\n h4. Useful Component Readiness Links:\n"
		description += "\nWe are proving the following two links for your convenience:\n"
		description += fmt.Sprintf("\n- Click [here|%s] to access a snapshot of the component readiness page at the time this issue was generated.\n", absURL)
		description += fmt.Sprintf("\n- Click [here|%s] to access the component readiness page with latest data. This is useful for developers to verify their fixes.\n", viewURL)

		description += "\n h4. Workflow Requirement:\n"
		description += "\n This is an automatically generated Jira card against the component based on stats generated from component readiness dashboard. Please follow the following requirements when dealing with this card:\n"
		description += "\n * Please use this card as a placeholder for all the regressed tests for your component. A separate issue should be created for each regressed test and linked to this issue. Please only close this issue when all known regressed tests are believed fixed.\n"
		description += "\n * Please do not remove 'Release Blocker' label. The bot will automatically add it back if any regressed tests continue showing for the component.\n"

		summary := fmt.Sprintf("Component Readiness: %s test regressed", component.Component)
		issue := jira.Issue{
			Fields: &jira.IssueFields{
				Description: description,
				Type: jira.IssueType{
					Name: "Bug",
				},
				Project: jira.Project{
					Key: component.Project,
				},
				Components: []*jira.Component{
					{
						Name: component.Component,
					},
				},
				Summary: summary,
				AffectsVersions: []*jira.AffectsVersion{
					{
						Name: view.SampleRelease.Name,
					},
				},
				Labels: []string{jiratype.LabelJiraAutomator},
			},
		}
		if !j.dryRun {
			created, err := j.jiraClient.CreateIssue(&issue)
			if err != nil {
				return err
			}
			// Set Release Blocker field. Jira does not allow setting those during creation. So do it in separate step.
			return j.updateReleaseBlocker(created, view.SampleRelease.Name)
		}
		issueStr, err := json.MarshalIndent(issue, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "\n====================================================================\n")
		fmt.Fprintf(os.Stdout, "Creating the following jira issue\n%s", issueStr)
		fmt.Fprintf(os.Stdout, "\n====================================================================\n")
	}
	return nil
}

func (j JiraAutomator) updateJiraIssueForRegressions(issue jira.Issue, view crview.View, component JiraComponent, tests []crtype.ReportTestSummary) error {
	switch issue.Fields.Status.Name {
	case jiratype.StatusNew, jiratype.StatusInProgress, jiratype.StatusAssigned, jiratype.StatusModified:
		// New/Assigned/In Progress/Modified
		err := j.updateExistingJiraIssue(view, &issue)
		if err != nil {
			return err
		}
	case jiratype.StatusOnQA, jiratype.StatusVerified, jiratype.StatusClosed:
		// QA/Verified/Closed
		resolutionDate := time.Time(issue.Fields.Resolutiondate)
		if view.SampleRelease.Start.After(resolutionDate) {
			// Existing issue does not cover current regression
			err := j.createNewJiraIssueForRegressions(view, component, tests, nil)
			if err != nil {
				return err
			}
		} else {
			// Overlap between current analysis and jira card fix. Do two more analysis:
			// a. Scope Check: Run with a sample start date of resolutionDate-2 weeks and end resolutionDate.
			//    If current analysis contains new tests not covered by scope check, create new card.
			// b. Fix Check: Run with a sample start date of resolutionDate and the original end date. Only
			//    do this after a reasonable number of days has passed.
			scopeView := view
			scopeView.TestIDOption.Component = component.Component
			scopeView.SampleRelease.RelativeStart = resolutionDate.Add(-14 * time.Hour * 24).Format(time.RFC3339)
			scopeView.SampleRelease.RelativeEnd = resolutionDate.Format(time.RFC3339)
			scopeReport, err := j.getComponentReportForView(scopeView)
			if err != nil {
				return err
			}

			// Identify tests only appearing in current report, not scope report
			scopeRegressedTests := map[crtest.RowIdentification]map[crtest.ColumnID]crtype.ReportTestSummary{}
			for _, row := range scopeReport.Rows {
				for _, col := range row.Columns {
					for _, test := range col.RegressedTests {
						if _, ok := scopeRegressedTests[test.RowIdentification]; !ok {
							scopeRegressedTests[row.RowIdentification] = map[crtest.ColumnID]crtype.ReportTestSummary{}
						}
						columnKeyBytes, err := json.Marshal(test.ColumnIdentification)
						if err != nil {
							return err
						}
						scopeRegressedTests[test.RowIdentification][crtest.ColumnID(columnKeyBytes)] = test
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
				} else if _, ok := scopeRegressedTests[test.RowIdentification][crtest.ColumnID(columnKeyBytes)]; !ok {
					newTests = append(newTests, test)
				}
			}
			if len(newTests) > 0 {
				// Any tests not covered by scope check is considered new
				err := j.createNewJiraIssueForRegressions(view, component, newTests, &issue)
				if err != nil {
					return err
				}
			} else if resolutionDate.Add(fixCheckWaitPeriod).Before(view.SampleRelease.End) {
				// This means scope report contains all tests from current report, verify fix
				fixView := view
				fixView.TestIDOption.Component = component.Component
				fixView.SampleRelease.RelativeStart = resolutionDate.Format(time.RFC3339)
				fixReport, err := j.getComponentReportForView(fixView)
				if err != nil {
					return err
				}
				regressedTests, err := j.groupRegressedTestsByComponents(fixReport)
				if err != nil {
					return err
				}
				if tests, ok := regressedTests[component]; ok {
					err := j.createNewJiraIssueForRegressions(fixView, component, tests, nil)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (j JiraAutomator) updateReleaseBlocker(issue *jira.Issue, release string) error {
	if j.isPreRelease(release) {
		unknowns := tcontainer.NewMarshalMap()
		unknowns[jiratype.CustomFieldReleaseBlockerName] = map[string]string{"value": jiratype.CustomFieldReleaseBlockerValueApproved}
		issue := jira.Issue{
			Key: issue.Key,
			Fields: &jira.IssueFields{
				Unknowns: unknowns,
			},
		}
		if !j.dryRun {
			_, err := j.jiraClient.UpdateIssue(&issue)
			return err
		}
		fmt.Fprintf(os.Stdout, "\n====================================================================\n")
		fmt.Fprintf(os.Stdout, "Updating Release Blocker for %s", issue.ID)
		fmt.Fprintf(os.Stdout, "\n====================================================================\n")
	}
	return nil
}

// groupRegressedTestsByComponents groups the regressed tests in the report by components.
// It also takes into consideration a special column grouping. The idea is if a certain column variant (e.g. metal) has more
// red cells, we will not want to create a jira card for all the components affected. Instead, we will just create
// one jira card for metal platform.
func (j JiraAutomator) groupRegressedTestsByComponents(report crtype.ComponentReport) (map[JiraComponent][]crtype.ReportTestSummary, error) {
	componentRegressedTests := map[JiraComponent][]crtype.ReportTestSummary{}
	columnToRegressionCount := map[string]int{}
	columnToVariantsToThreshold := map[string]map[Variant]int{}
	// First we count the number of red cells for each column
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			if len(col.RegressedTests) == 0 {
				continue
			}
			columnKeyBytes, err := json.Marshal(col.ColumnIdentification)
			if err != nil {
				return componentRegressedTests, err
			}
			columnID := string(columnKeyBytes)
			columnToRegressionCount[columnID]++

			// find all the defined columnThresholds this column is relevant to
			for k, n := range col.Variants {
				v := Variant{Name: k, Value: n}
				if threshold, ok := j.columnThresholds[v]; ok {
					columnToVariantsToThreshold[columnID] = map[Variant]int{v: threshold}
				}
			}
		}
	}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			if len(col.RegressedTests) == 0 {
				continue
			}
			columnKeyBytes, err := json.Marshal(col.ColumnIdentification)
			if err != nil {
				return componentRegressedTests, err
			}
			columnID := string(columnKeyBytes)
			useVariantBasedComponent := false
			if variantToThreshold, ok := columnToVariantsToThreshold[columnID]; ok {
				for variant, threshold := range variantToThreshold {
					if threshold < columnToRegressionCount[columnID] {
						if component, ok := j.variantToJiraComponents[variant]; ok {
							componentRegressedTests[component] = append(componentRegressedTests[component], col.RegressedTests...)
							useVariantBasedComponent = true
						}
					}
				}
			}
			if !useVariantBasedComponent {
				jc := JiraComponent{Project: jiratype.ProjectKeyOCPBugs, Component: row.Component}
				componentRegressedTests[jc] = append(componentRegressedTests[jc], col.RegressedTests...)
			}
		}
	}
	for component, tests := range componentRegressedTests {
		sort.Slice(tests, func(i, j int) bool {
			return tests[i].ReportStatus < tests[j].ReportStatus
		})
		componentRegressedTests[component] = tests
	}
	return componentRegressedTests, nil
}

func (j JiraAutomator) automateJirasForView(view crview.View) error {

	logger := log.WithField("view", view.Name)
	logger.Info("automate jiras for view")

	report, err := j.getComponentReportForView(view)
	if err != nil {
		logger.WithError(err).Error("error getting report for view")
		return err
	}

	componentRegressedTests, err := j.groupRegressedTestsByComponents(report)
	if err != nil {
		logger.WithError(err).Error("error getting regressed tests from report")
		return err
	}
	for component, tests := range componentRegressedTests {
		// fetch jira bugs
		if j.includeComponents.Len() > 0 && !j.includeComponents.Has(component.Project+":"+component.Component) {
			continue
		}
		issues, err := j.getExistingIssuesForComponent(view, component)
		if err != nil {
			log.WithError(err).Error("error getting existing jira issues")
		}

		// No existing issues, create new one
		if len(issues) == 0 {
			err := j.createNewJiraIssueForRegressions(view, component, tests, nil)
			if err != nil {
				log.WithError(err).Error("error creating jira issue")
			}
		} else {
			selected := issues[0]
			err := j.updateJiraIssueForRegressions(selected, view, component, tests)
			if err != nil {
				log.WithError(err).Error("error updating jira issue")
			}
		}
	}
	return nil
}

func (j JiraAutomator) Run() error {
	log.Infof("Start automating jiras for component readiness regressions")
	for _, view := range j.views {
		err := j.automateJirasForView(view)
		if err != nil {
			return err
		}
	}
	log.Infof("Done automating jiras for component readiness regressions")
	return nil
}

func GetVariantJiraMap(ctx context.Context, bqClient *bqclient.Client) (map[Variant]JiraComponent, error) {
	result := map[Variant]JiraComponent{}

	queryString := "SELECT * FROM openshift-gce-devel.ci_analysis_us.variant_mapping_latest"
	q := bqClient.BQ.Query(queryString)
	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying variant mapping data from bigquery")
		return result, err
	}

	for {
		r := v1.VariantMapping{}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing variant mapping row from bigquery")
			return result, err
		}
		result[Variant{Name: r.VariantName, Value: r.VariantValue}] = JiraComponent{Project: r.JiraProject, Component: r.JiraComponent}
	}
	return result, nil
}
