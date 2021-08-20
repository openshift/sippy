package stepmetricshtml

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
)

func PrintLandingPage(tr TableRequest, timestamp time.Time) string {
	sb := strings.Builder{}

	release := tr.request().Release

	fmt.Fprintf(&sb, generichtml.HTMLPageStart, "Step Metrics For "+release)

	fmt.Fprintln(&sb, renderTables(allMultistagesTable(tr)))
	fmt.Fprintln(&sb, renderTables(allStepsTable(tr)))

	fmt.Fprintf(&sb, generichtml.HTMLPageEnd, timestamp.Format("Jan 2 15:04 2006 MST"))

	return sb.String()
}

func RenderRequest(tr TableRequest, timestamp time.Time) (string, error) {
	tables, err := getTablesForTableRequest(tr)
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}

	title := tables[0].pageTitle

	if tr.request().JobName == stepmetrics.All {
		title = "All Step Metrics By Job Name"
	}

	header := generichtml.HTMLElement{
		Element: "h1",
		Text:    title,
		Params: map[string]string{
			"class": "text-center",
		},
	}

	fmt.Fprintf(&sb, generichtml.HTMLPageStart, title)
	fmt.Fprintln(&sb, header.ToHTML())

	for _, table := range tables {
		fmt.Fprintln(&sb, table.toHTML())
	}

	fmt.Fprintf(&sb, generichtml.HTMLPageEnd, timestamp.Format("Jan 2 15:04 2006 MST"))

	return sb.String(), nil
}

func getTablesForTableRequest(tr TableRequest) ([]tableOpts, error) {
	req := tr.request()

	if req.MultistageJobName != "" {
		if req.MultistageJobName == stepmetrics.All {
			return allMultistagesTable(tr), nil
		}

		return multistageDetailTable(tr), nil
	}

	if req.StepName != "" {
		if req.StepName == stepmetrics.All {
			return allStepsTable(tr), nil
		}

		return stepDetailTable(tr), nil
	}

	if req.JobName != "" {
		if req.JobName == stepmetrics.All {
			return allJobsTable(tr), nil
		}
		return byJobTable(tr), nil
	}

	return []tableOpts{}, fmt.Errorf("unknown table for request")
}

func AllMultistages(curr, prev sippyprocessingv1.TestReport) string {
	tr := newTableRequestWithRelease(curr, prev)
	return renderTables(allMultistagesTable(tr))
}

func allMultistagesTable(tr TableRequest) []tableOpts {
	req := tr.request()

	opts := tableOpts{
		pageTitle:   "All Multistage Job Names For " + req.Release,
		width:       "4",
		title:       "All Multistage Job Names",
		description: "All Multistage Job Names",
		headerRows: []generichtml.HTMLTableRow{
			getMultistageHeaderRow(),
		},
		rows: []tableRow{},
	}

	for _, multistageName := range tr.allMultistageNames() {
		currByMultistage, prevByMultistage := tr.byMultistageName(multistageName)

		detail := stepmetrics.NewStepDetail(
			currByMultistage.Aggregated,
			prevByMultistage.Aggregated,
		)

		opts.rows = append(opts.rows, tableRow{
			name:  multistageName,
			trend: detail.Trend,
			ciSearchURL: &CISearchURL{
				Release: req.Release,
				Search:  fmt.Sprintf(`operator.Run multi-stage test %s`, multistageName),
			},
			sippyURL: &SippyURL{
				Release:           req.Release,
				MultistageJobName: multistageName,
			},
			stepRegistryURL: &StepRegistryURL{
				Search: multistageName,
			},
		})
	}

	return []tableOpts{opts}
}

func AllSteps(curr, prev sippyprocessingv1.TestReport) string {
	tr := newTableRequestWithRelease(curr, prev)
	return renderTables(allStepsTable(tr))
}

func allStepsTable(tr TableRequest) []tableOpts {
	opts := tableOpts{
		pageTitle:   "Step Metrics For All Steps",
		title:       "Frequency of passes / failures for step registry items for all steps",
		description: "Step Metrics For All Steps",
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getStepNameHeaderRow(),
		},
		rows: []tableRow{},
	}

	req := tr.request()

	for _, stageName := range tr.allStageNames() {
		currSteps, prevSteps := tr.byStageName(stageName)

		stepDetails := stepmetrics.NewStepDetail(currSteps.Aggregated, prevSteps.Aggregated)

		opts.rows = append(opts.rows, tableRow{
			name:  stageName,
			trend: stepDetails.Trend,
			sippyURL: &SippyURL{
				Release:  req.Release,
				StepName: stepDetails.Name,
			},
			ciSearchURL: &CISearchURL{
				Release:     req.Release,
				SearchRegex: fmt.Sprintf(`operator\.Run multi-stage test .*-%s container test`, stepDetails.Current.Name),
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetails.Name,
			},
		})
	}

	return []tableOpts{opts}
}

func AllJobs(tr TableRequest) string {
	return renderTables(allJobsTable(tr))
}

func allJobsTable(tr TableRequest) []tableOpts {
	req := tr.request()

	opts := []tableOpts{}

	for _, jobName := range tr.allJobNames() {
		detail := jobStepDetail(tr.withRequest(stepmetrics.Request{
			Release: req.Release,
			JobName: jobName,
		}))

		opts = append(opts, detail)
	}

	return opts
}

func byJobTable(tr TableRequest) []tableOpts {
	return []tableOpts{
		getMultistageAggregateDetailForJob(tr),
		jobStepDetail(tr),
	}
}

func renderTables(opts []tableOpts) string {
	sb := strings.Builder{}

	for _, opt := range opts {
		fmt.Fprintln(&sb, opt.toHTML())
	}

	return sb.String()
}

func stepDetailTable(tr TableRequest) []tableOpts {
	req := tr.request()

	opts := tableOpts{
		pageTitle:   "Step Metrics For " + req.StepName,
		title:       "Frequency of passes / failures for " + req.StepName + " by multistage job name",
		description: "Step Metrics For " + req.StepName + " By Multistage Job Name",
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getMultistageHeaderRow(),
		},
		rows: []tableRow{},
	}

	currByStageName, prevByStageName := tr.byStageName(req.StepName)

	for _, multistageName := range getSortedKeys(currByStageName.ByMultistageName) {
		currStage := currByStageName.ByMultistageName[multistageName]

		multistageDetail := stepmetrics.NewStepDetail(
			currStage,
			prevByStageName.ByMultistageName[multistageName],
		)

		opts.rows = append(opts.rows, tableRow{
			name:  multistageName,
			trend: multistageDetail.Trend,
			sippyURL: &SippyURL{
				Release:           req.Release,
				MultistageJobName: multistageName,
			},
			stepRegistryURL: &StepRegistryURL{
				Search: multistageName,
			},
			ciSearchURL: &CISearchURL{
				Release: req.Release,
				Search:  currStage.OriginalTestName,
			},
		})
	}

	return []tableOpts{opts}
}

func multistageDetailTable(tr TableRequest) []tableOpts {
	return []tableOpts{
		getMultistageAggregate(tr),
		getMultistageDetail(tr),
	}
}

func getMultistageAggregate(tr TableRequest) tableOpts {
	req := tr.request()

	currByMultistage, prevByMultistage := tr.byMultistageName(req.MultistageJobName)

	multistageDetail := stepmetrics.NewStepDetail(
		currByMultistage.Aggregated,
		prevByMultistage.Aggregated,
	)

	return tableOpts{
		pageTitle:   "Multistage Job Detail For " + req.MultistageJobName,
		title:       "Overall Multistage Metrics For " + req.MultistageJobName,
		description: "Overall Multistage Metrics For " + req.MultistageJobName,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getMultistageHeaderRow(),
		},
		rows: []tableRow{
			{
				name:  multistageDetail.Name,
				trend: multistageDetail.Trend,
				stepRegistryURL: &StepRegistryURL{
					Search: multistageDetail.Name,
				},
			},
		},
	}
}

func getMultistageDetail(tr TableRequest) tableOpts {
	req := tr.request()

	opts := tableOpts{
		pageTitle:   "All Step Names for Multistage Job " + req.MultistageJobName,
		title:       "All Step Names for Multistage Job " + req.MultistageJobName,
		description: "All Step Names For Multistage Job " + req.MultistageJobName,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getStepNameHeaderRow(),
		},
		rows: []tableRow{},
	}

	currByMultistage, prevByMultistage := tr.byMultistageName(req.MultistageJobName)

	for _, stepName := range getSortedKeys(currByMultistage.StageResults) {
		stepDetail := stepmetrics.NewStepDetail(
			currByMultistage.StageResults[stepName],
			prevByMultistage.StageResults[stepName],
		)

		opts.rows = append(opts.rows, tableRow{
			name:  stepName,
			trend: stepDetail.Trend,
			sippyURL: &SippyURL{
				Release:  req.Release,
				StepName: stepDetail.Name,
			},
			ciSearchURL: &CISearchURL{
				Release: req.Release,
				Search:  stepDetail.Current.OriginalTestName,
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetail.Name,
			},
		})
	}

	return opts
}

func getMultistageAggregateDetailForJob(tr TableRequest) tableOpts {
	req := tr.request()

	currByJobName, prevByJobName := tr.byJobName(req.JobName)

	multistageDetail := stepmetrics.NewStepDetail(
		currByJobName.Aggregated,
		prevByJobName.Aggregated,
	)

	return tableOpts{
		pageTitle:   "Multistage Job Detail For " + req.JobName,
		title:       "Overall Multistage Metrics For " + req.JobName + " With Multistage " + multistageDetail.Name,
		description: "Overall Multistage Metrics For " + req.JobName + " With Multistage " + multistageDetail.Name,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getMultistageHeaderRow(),
		},
		rows: []tableRow{
			{
				name:  multistageDetail.Name,
				trend: multistageDetail.Trend,
				stepRegistryURL: &StepRegistryURL{
					Search: multistageDetail.Name,
				},
			},
		},
	}
}

func jobStepDetail(tr TableRequest) tableOpts {
	req := tr.request()

	opts := tableOpts{
		pageTitle:   "Step Metrics For Job " + req.JobName,
		title:       "Step Metrics For Job " + req.JobName,
		description: "Step Metrics For " + req.JobName,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getStepNameHeaderRow(),
		},
	}

	currByJobName, prevByJobName := tr.byJobName(req.JobName)

	for _, stepName := range getSortedKeys(currByJobName.StageResults) {
		stepDetail := stepmetrics.NewStepDetail(
			currByJobName.StageResults[stepName],
			prevByJobName.StageResults[stepName])

		opts.rows = append(opts.rows, tableRow{
			name:  stepName,
			trend: stepDetail.Trend,
			sippyURL: &SippyURL{
				Release:  req.Release,
				StepName: stepDetail.Name,
			},
			ciSearchURL: &CISearchURL{
				Search:   stepDetail.Current.OriginalTestName,
				JobRegex: fmt.Sprintf("^%s$", regexp.QuoteMeta(req.JobName)),
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetail.Name,
			},
		})
	}

	return opts
}
