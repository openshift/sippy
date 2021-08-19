package stepmetricshtml

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/util/sets"
)

func PrintLandingPage(curr, prev sippyprocessingv1.TestReport) (string, error) {
	sb := strings.Builder{}

	fmt.Fprintf(&sb, generichtml.HTMLPageStart, "Step Metrics For "+curr.Release)

	api := stepmetrics.NewStepMetricsAPI(curr, prev)

	allMultistagesResp, err := api.Fetch(stepmetrics.Request{
		Release:           curr.Release,
		MultistageJobName: stepmetrics.All,
	})

	if err != nil {
		return "", err
	}

	allStepsResp, err := api.Fetch(stepmetrics.Request{
		Release:  curr.Release,
		StepName: stepmetrics.All,
	})

	if err != nil {
		return "", err
	}

	fmt.Fprintln(&sb, allMultistages(allMultistagesResp).toHTML())
	fmt.Fprintln(&sb, allSteps(allStepsResp).toHTML())

	fmt.Fprintf(&sb, generichtml.HTMLPageEnd, curr.Timestamp.Format("Jan 2 15:04 2006 MST"))

	return sb.String(), nil
}

func RenderResponse(resp stepmetrics.Response, timestamp time.Time) (string, error) {
	tables, err := getTablesForResponse(resp)
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}

	title := tables[0].pageTitle

	if resp.Request.JobName == stepmetrics.All {
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

func getTablesForResponse(resp stepmetrics.Response) ([]tableOpts, error) {
	if resp.Request.MultistageJobName != "" {
		if resp.Request.MultistageJobName == stepmetrics.All {
			return []tableOpts{allMultistages(resp)}, nil
		}

		return multistageDetail(resp), nil
	}

	if resp.Request.StepName != "" {
		if resp.Request.StepName == stepmetrics.All {
			return []tableOpts{allSteps(resp)}, nil
		}

		return stepDetail(resp), nil
	}

	if resp.Request.JobName != "" {
		if resp.Request.JobName == stepmetrics.All {
			return allJobs(resp)
		}
		return byJob(resp)
	}

	return []tableOpts{}, fmt.Errorf("unknown table for response")
}

func AllMultistages(resp stepmetrics.Response) (string, error) {
	if resp.Request.MultistageJobName == "" {
		return "", fmt.Errorf("multistage job name empty")
	}

	if resp.Request.MultistageJobName != stepmetrics.All {
		return "", fmt.Errorf("multistage job name must be \"All\"")
	}

	return allMultistages(resp).toHTML(), nil
}

func allMultistages(resp stepmetrics.Response) tableOpts {
	opts := tableOpts{
		pageTitle:   "All Multistage Job Names For " + resp.Request.Release,
		width:       "4",
		title:       "All Multistage Job Names",
		description: "All Multistage Job Names",
		headerRows: []generichtml.HTMLTableRow{
			getMultistageHeaderRow(),
		},
		rows: []tableRow{},
	}

	sortedMultistageNames := sets.StringKeySet(resp.MultistageDetails).List()

	for _, multistageName := range sortedMultistageNames {
		multistageDetail := resp.MultistageDetails[multistageName]

		opts.rows = append(opts.rows, tableRow{
			name:  multistageDetail.Name,
			trend: multistageDetail.Trend,
			ciSearchURL: &CISearchURL{
				Release: resp.Request.Release,
				Search:  fmt.Sprintf(`operator.Run multi-stage test %s`, multistageDetail.Name),
			},
			sippyURL: &SippyURL{
				Release:           resp.Request.Release,
				MultistageJobName: multistageDetail.Name,
			},
			stepRegistryURL: &StepRegistryURL{
				Search: multistageDetail.Name,
			},
		})
	}

	return opts
}

func MultistageDetail(resp stepmetrics.Response) (string, error) {
	if resp.Request.MultistageJobName == "" {
		return "", fmt.Errorf("multistage job name empty")
	}

	if resp.Request.MultistageJobName == stepmetrics.All {
		return "", fmt.Errorf("multistage job name cannot be \"All\"")
	}

	sb := strings.Builder{}
	for _, opts := range multistageDetail(resp) {
		fmt.Fprintln(&sb, opts.toHTML())
	}

	return sb.String(), nil
}

func multistageDetail(resp stepmetrics.Response) []tableOpts {
	multistageDetails := resp.MultistageDetails[resp.Request.MultistageJobName]

	opts := tableOpts{
		pageTitle:   "All Step Names for Multistage Job " + multistageDetails.Name,
		title:       "All Step Names for Multistage Job " + multistageDetails.Name,
		description: "All Step Names For Multistage Job " + multistageDetails.Name,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getStepNameHeaderRow(),
		},
		rows: []tableRow{},
	}

	sortedStepNames := sets.StringKeySet(multistageDetails.StepDetails).List()

	for _, stepName := range sortedStepNames {
		stepDetail := multistageDetails.StepDetails[stepName]

		opts.rows = append(opts.rows, tableRow{
			name:  stepName,
			trend: stepDetail.Trend,
			sippyURL: &SippyURL{
				Release:  resp.Request.Release,
				StepName: stepDetail.Name,
			},
			ciSearchURL: &CISearchURL{
				Release: resp.Request.Release,
				Search:  stepDetail.Current.OriginalTestName,
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetail.Name,
			},
		})
	}

	return []tableOpts{
		multistageAggregateDetail(multistageDetails),
		opts,
	}
}

func AllSteps(resp stepmetrics.Response) (string, error) {
	if resp.Request.StepName == "" {
		return "", fmt.Errorf("step name empty")
	}

	if resp.Request.StepName != stepmetrics.All {
		return "", fmt.Errorf("step name must be \"All\"")
	}

	return allSteps(resp).toHTML(), nil
}

func allSteps(resp stepmetrics.Response) tableOpts {
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

	sortedStageNames := sets.StringKeySet(resp.StepDetails).List()

	for _, stageName := range sortedStageNames {
		stepDetails := resp.StepDetails[stageName]

		opts.rows = append(opts.rows, tableRow{
			name:  stageName,
			trend: stepDetails.Trend,
			sippyURL: &SippyURL{
				Release:  resp.Request.Release,
				StepName: stepDetails.Name,
			},
			ciSearchURL: &CISearchURL{
				Release:     resp.Request.Release,
				SearchRegex: fmt.Sprintf(`operator\.Run multi-stage test .*-%s container test`, stepDetails.Current.Name),
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetails.Name,
			},
		})
	}

	return opts
}

func StepDetail(resp stepmetrics.Response) (string, error) {
	if resp.Request.StepName == "" {
		return "", fmt.Errorf("step name empty")
	}

	if resp.Request.StepName == stepmetrics.All {
		return "", fmt.Errorf("step name must not be \"All\"")
	}

	sb := strings.Builder{}
	for _, table := range stepDetail(resp) {
		fmt.Fprintln(&sb, table.toHTML())
	}

	return sb.String(), nil
}

func stepDetail(resp stepmetrics.Response) []tableOpts {
	stepName := resp.Request.StepName

	opts := tableOpts{
		pageTitle:   "Step Metrics For " + stepName,
		title:       "Frequency of passes / failures for " + stepName + " by multistage job name",
		description: "Step Metrics For " + stepName + " By Multistage Job Name",
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getMultistageHeaderRow(),
		},
		rows: []tableRow{},
	}

	sortedMultistageNames := sets.StringKeySet(resp.StepDetails[stepName].ByMultistage).List()

	stageResult := resp.StepDetails[stepName]

	for _, multistageName := range sortedMultistageNames {
		multistageResult := stageResult.ByMultistage[multistageName]

		opts.rows = append(opts.rows, tableRow{
			name:  multistageName,
			trend: multistageResult.Trend,
			sippyURL: &SippyURL{
				Release:           resp.Request.Release,
				MultistageJobName: multistageName,
			},
			stepRegistryURL: &StepRegistryURL{
				Search: multistageName,
			},
			ciSearchURL: &CISearchURL{
				Release: resp.Request.Release,
				Search:  stageResult.ByMultistage[multistageName].Current.OriginalTestName,
			},
		})
	}

	return []tableOpts{
		opts,
	}
}

func AllJobs(resp stepmetrics.Response) (string, error) {
	jobs, err := allJobs(resp)
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}

	for _, job := range jobs {
		fmt.Fprintln(&sb, job.toHTML())
	}

	return sb.String(), nil
}

func allJobs(resp stepmetrics.Response) ([]tableOpts, error) {
	sortedJobNames := sets.StringKeySet(resp.JobDetails).List()

	tables := []tableOpts{}

	for _, jobName := range sortedJobNames {
		jobDetails := resp.JobDetails[jobName]
		tables = append(tables, jobStepDetail(resp, jobDetails))
	}

	return tables, nil
}

func ByJob(resp stepmetrics.Response) (string, error) {
	opts, err := byJob(resp)
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}

	for _, opt := range opts {
		fmt.Fprintln(&sb, opt.toHTML())
	}

	return sb.String(), nil
}

func byJob(resp stepmetrics.Response) ([]tableOpts, error) {
	if resp.Request.JobName == "" {
		return []tableOpts{}, fmt.Errorf("job name empty")
	}

	if len(resp.JobDetails) != 1 {
		return []tableOpts{}, fmt.Errorf("expected a single job")
	}

	jobName := sets.StringKeySet(resp.JobDetails).UnsortedList()[0]

	jobDetails := resp.JobDetails[jobName]

	opts := []tableOpts{
		jobStepDetail(resp, jobDetails),
		multistageAggregateDetailForJob(jobDetails.MultistageDetails, jobName),
	}

	return opts, nil
}

func multistageAggregateDetailForJob(multistageDetail stepmetrics.MultistageDetails, jobName string) tableOpts {
	return tableOpts{
		pageTitle:   "Multistage Job Detail For " + jobName,
		title:       "Overall Multistage Metrics For " + jobName + " With Multistage " + multistageDetail.Name,
		description: "Overall Multistage Metrics For " + jobName + " With Multistage " + multistageDetail.Name,
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

func multistageAggregateDetail(multistageDetail stepmetrics.MultistageDetails) tableOpts {
	return tableOpts{
		pageTitle:   "Multistage Job Detail For " + multistageDetail.Name,
		title:       "Overall Multistage Metrics For " + multistageDetail.Name,
		description: "Overall Multistage Metrics For " + multistageDetail.Name,
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

func jobStepDetail(resp stepmetrics.Response, jobDetails stepmetrics.JobDetails) tableOpts {
	opts := tableOpts{
		pageTitle:   "Step Metrics For Job " + jobDetails.JobName,
		title:       "Step Metrics For Job " + jobDetails.JobName,
		description: "Step Metrics For " + jobDetails.JobName,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getStepNameHeaderRow(),
		},
	}

	sortedStepNames := sets.StringKeySet(jobDetails.StepDetails).List()

	for _, stepName := range sortedStepNames {
		stepDetail := jobDetails.StepDetails[stepName]

		opts.rows = append(opts.rows, tableRow{
			name:  stepName,
			trend: stepDetail.Trend,
			sippyURL: &SippyURL{
				Release:  resp.Request.Release,
				StepName: stepDetail.Name,
			},
			ciSearchURL: &CISearchURL{
				Search:   stepDetail.Current.OriginalTestName,
				JobRegex: fmt.Sprintf("^%s$", regexp.QuoteMeta(jobDetails.JobName)),
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetail.Name,
			},
		})
	}

	return opts
}
