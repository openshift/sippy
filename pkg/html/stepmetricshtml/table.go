package stepmetricshtml

import (
	"fmt"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/util/sets"
)

func RenderResponse(resp stepmetrics.Response, timestamp time.Time) (string, error) {
	tables, err := getTablesForResponse(resp)
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}

	header := generichtml.HTMLElement{
		Element: "h1",
		Text:    tables[0].pageTitle,
		Params: map[string]string{
			"class": "text-center",
		},
	}

	fmt.Fprintf(&sb, generichtml.HTMLPageStart, tables[0].pageTitle)
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

		return []tableOpts{stepDetail(resp)}, nil
	}

	if resp.Request.JobName != "" {
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
	multistageJobName := resp.Request.MultistageJobName

	opts := tableOpts{
		pageTitle:   "All Step Names for Multistage Job " + multistageJobName,
		title:       "All Step Names for Multistage Job " + multistageJobName,
		description: "All Step Names For Multistage Job " + multistageJobName,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getStepNameHeaderRow(),
		},
		rows: []tableRow{},
	}

	sortedStepNames := sets.StringKeySet(resp.MultistageDetails[multistageJobName].StepDetails).List()

	for _, stepName := range sortedStepNames {
		stepDetail := resp.MultistageDetails[multistageJobName].StepDetails[stepName]

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
		opts,
		multistageAggregateDetail(resp, multistageJobName),
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
				Release: resp.Request.Release,
				Search:  stepDetails.Current.OriginalTestName,
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

	return stepDetail(resp).toHTML(), nil
}

func stepDetail(resp stepmetrics.Response) tableOpts {
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

	return opts
}

func ByJob(resp stepmetrics.Response) (string, error) {
	opts, err := byJob(resp)
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}

	for _, opt := range opts {
		sb.WriteString(opt.toHTML())
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func byJob(resp stepmetrics.Response) ([]tableOpts, error) {
	if resp.Request.JobName == "" {
		return []tableOpts{}, fmt.Errorf("job name empty")
	}

	if len(resp.MultistageDetails) > 1 {
		return []tableOpts{}, fmt.Errorf("jobs should only have a single multistage")
	}

	multistageName := sets.StringKeySet(resp.MultistageDetails).UnsortedList()[0]

	opts := []tableOpts{
		jobStepDetail(resp, multistageName),
		multistageAggregateDetail(resp, multistageName),
	}

	return opts, nil
}

func multistageAggregateDetail(resp stepmetrics.Response, multistageName string) tableOpts {
	multistageDetail := resp.MultistageDetails[multistageName]

	return tableOpts{
		pageTitle:   "Multistage Job Detail For " + resp.Request.JobName,
		title:       "Overall Multistage Metrics For " + resp.Request.JobName + " With Multistage " + multistageName,
		description: "Overall Multistage Metrics For " + resp.Request.JobName + " With Multistage " + multistageName,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getMultistageHeaderRow(),
		},
		rows: []tableRow{
			{
				name:  multistageName,
				trend: multistageDetail.Trend,
				stepRegistryURL: &StepRegistryURL{
					Search: multistageName,
				},
			},
		},
	}
}

func jobStepDetail(resp stepmetrics.Response, multistageName string) tableOpts {
	jobName := resp.Request.JobName

	opts := tableOpts{
		pageTitle:   "Step Metrics For Job " + jobName,
		title:       "Step Metrics For Job " + jobName,
		description: "Step Metrics For " + jobName,
		width:       "4",
		headerRows: []generichtml.HTMLTableRow{
			getStepNameHeaderRow(),
		},
	}

	stepDetails := resp.MultistageDetails[multistageName].StepDetails

	sortedStepNames := sets.StringKeySet(stepDetails).List()

	for _, stepName := range sortedStepNames {
		stepDetail := stepDetails[stepName]

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

	return opts
}
