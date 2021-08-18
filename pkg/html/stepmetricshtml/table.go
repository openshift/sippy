package stepmetricshtml

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/util/sets"
)

type tableRow struct {
	name            string
	trend           stepmetrics.Trend
	sippyURL        *SippyURL
	ciSearchURL     *CISearchURL
	stepRegistryURL *StepRegistryURL
}

func (t tableRow) toHTMLTableRow() generichtml.HTMLTableRow {
	row := generichtml.NewHTMLTableRow(map[string]string{})

	nameItems := []generichtml.HTMLItem{
		generichtml.NewHTMLTextElement(t.name),
	}

	if t.sippyURL != nil {
		nameItems = append(nameItems, getEnclosedHTMLLink("Detail", t.sippyURL))
	}

	if t.ciSearchURL != nil {
		nameItems = append(nameItems, getEnclosedHTMLLink("CI Search", t.ciSearchURL))
	}

	if t.stepRegistryURL != nil {
		nameItems = append(nameItems, getEnclosedHTMLLink("Step Registry", t.stepRegistryURL))
	}

	row.AddItems([]generichtml.HTMLItem{
		generichtml.HTMLTableRowItem{
			HTMLItems: generichtml.SpaceHTMLItems(nameItems),
		},
		generichtml.HTMLTableRowItem{
			Text: getArrowForTrend(t.trend),
		},
		generichtml.HTMLTableRowItem{
			Text: getStageResultDetail(t.trend.Current),
		},
		generichtml.HTMLTableRowItem{
			Text: getStageResultDetail(t.trend.Previous),
		},
	})

	return row
}

type StepMetricsHTMLTable struct {
	release   string
	timestamp time.Time
}

func NewStepMetricsHTMLTable(release string, timestamp time.Time) StepMetricsHTMLTable {
	return StepMetricsHTMLTable{
		release:   release,
		timestamp: timestamp,
	}
}

func (s StepMetricsHTMLTable) RenderResponse(w http.ResponseWriter, resp stepmetrics.Response) error {
	table, err := s.getTableForResponse(resp)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, generichtml.HTMLPageStart, fmt.Sprintf("Release %s Step Metrics Dashboard", s.release))
	fmt.Fprint(w, table.ToHTML())
	fmt.Fprintf(w, generichtml.HTMLPageEnd, s.timestamp.Format("Jan 2 15:04 2006 MST"))

	return nil
}

func (s StepMetricsHTMLTable) getTableForResponse(resp stepmetrics.Response) (generichtml.HTMLTable, error) {
	if resp.Request.MultistageJobName != "" {
		return s.Multistage(resp)
	}

	if resp.Request.StepName != "" {
		return s.Stage(resp)
	}

	return generichtml.NewHTMLTable(map[string]string{}), fmt.Errorf("no table to render")
}

func (s StepMetricsHTMLTable) Multistage(resp stepmetrics.Response) (generichtml.HTMLTable, error) {
	multistageJobName := resp.Request.MultistageJobName

	if multistageJobName == stepmetrics.All {
		return s.AllMultistages(resp), nil
	}

	if multistageJobName != "" {
		return s.MultistageDetail(resp), nil
	}

	return generichtml.NewHTMLTable(map[string]string{}), fmt.Errorf("multistage job name required")
}

func (s StepMetricsHTMLTable) Stage(resp stepmetrics.Response) (generichtml.HTMLTable, error) {
	stepName := resp.Request.StepName

	if stepName == stepmetrics.All {
		return s.AllStages(resp), nil
	}

	if stepName != "" {
		return s.StageDetail(resp), nil
	}

	return generichtml.NewHTMLTable(map[string]string{}), fmt.Errorf("step name required")
}

func (s StepMetricsHTMLTable) AllMultistages(resp stepmetrics.Response) generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		width:       "4",
		title:       "All Multistage Job Names",
		description: "All Multistage Jobs",
	})

	table.AddHeaderRow(getMultistageHeaderRow())

	sortedMultistageNames := sets.StringKeySet(resp.MultistageDetails).List()

	for _, multistageName := range sortedMultistageNames {
		multistageDetail := resp.MultistageDetails[multistageName]

		tr := tableRow{
			name:  multistageDetail.Name,
			trend: multistageDetail.Trend,
			sippyURL: &SippyURL{
				Release:           s.release,
				MultistageJobName: multistageDetail.Name,
			},
			stepRegistryURL: &StepRegistryURL{
				Search: multistageDetail.Name,
			},
		}

		table.AddRow(tr.toHTMLTableRow())
	}

	return table
}

func (s StepMetricsHTMLTable) MultistageDetail(resp stepmetrics.Response) generichtml.HTMLTable {
	multistageJobName := resp.Request.MultistageJobName

	table := initializeTable(tableOpts{
		title:       "All Step Names for Multistage Job " + multistageJobName,
		description: "All Step Names For Multistage Job " + multistageJobName,
		width:       "4",
	})

	table.AddHeaderRow(getStepNameHeaderRow())

	sortedStepNames := sets.StringKeySet(resp.MultistageDetails[multistageJobName].StepDetails).List()

	for _, stepName := range sortedStepNames {
		stepDetail := resp.MultistageDetails[multistageJobName].StepDetails[stepName]

		tr := tableRow{
			name:  stepName,
			trend: stepDetail.Trend,
			sippyURL: &SippyURL{
				Release:  s.release,
				StepName: stepDetail.Name,
			},
			ciSearchURL: &CISearchURL{
				Release: s.release,
				Search:  stepDetail.Current.OriginalTestName,
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetail.Name,
			},
		}

		table.AddRow(tr.toHTMLTableRow())
	}

	return table
}

func (s StepMetricsHTMLTable) AllStages(resp stepmetrics.Response) generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		title:       "Frequency of passes / failures for step registry items for all steps",
		description: "Step Metrics For All Steps",
		width:       "4",
	})

	table.AddHeaderRow(getStepNameHeaderRow())

	sortedStageNames := sets.StringKeySet(resp.StepDetails).List()

	for _, stageName := range sortedStageNames {
		stepDetails := resp.StepDetails[stageName]

		tr := tableRow{
			name:  stageName,
			trend: stepDetails.Trend,
			sippyURL: &SippyURL{
				Release:  s.release,
				StepName: stepDetails.Name,
			},
			ciSearchURL: &CISearchURL{
				Release: s.release,
				Search:  stepDetails.Current.OriginalTestName,
			},
			stepRegistryURL: &StepRegistryURL{
				Reference: stepDetails.Name,
			},
		}

		table.AddRow(tr.toHTMLTableRow())
	}

	return table
}

func (s StepMetricsHTMLTable) StageDetail(resp stepmetrics.Response) generichtml.HTMLTable {
	stepName := resp.Request.StepName

	table := initializeTable(tableOpts{
		title:       "Frequency of passes / failures for " + stepName + " by multistage job name",
		description: "Step Metrics For " + stepName + " By Multistage Job Name",
		width:       "4",
	})

	table.AddHeaderRow(getMultistageHeaderRow())

	sortedMultistageNames := sets.StringKeySet(resp.StepDetails[stepName].ByMultistage).List()

	stageResult := resp.StepDetails[stepName]

	for _, multistageName := range sortedMultistageNames {
		multistageResult := stageResult.ByMultistage[multistageName]

		tr := tableRow{
			name:  multistageName,
			trend: multistageResult.Trend,
			sippyURL: &SippyURL{
				Release:           s.release,
				MultistageJobName: multistageName,
			},
			stepRegistryURL: &StepRegistryURL{
				Search: multistageName,
			},
			ciSearchURL: &CISearchURL{
				Release: s.release,
				Search:  stageResult.ByMultistage[multistageName].Current.OriginalTestName,
			},
		}

		table.AddRow(tr.toHTMLTableRow())
	}

	return table
}

func getStageResultDetail(stageResult sippyprocessingv1.StageResult) string {
	sb := strings.Builder{}
	fmt.Fprintf(&sb, "%0.2f", stageResult.PassPercentage)
	fmt.Fprint(&sb, "%")
	fmt.Fprintf(&sb, " (%d runs)", stageResult.Runs)
	return sb.String()
}

func getEnclosedHTMLLink(name string, linkURL URLGenerator) generichtml.HTMLItem {
	link := generichtml.NewHTMLLink(name, linkURL.URL())
	return generichtml.NewHTMLTextElement("(" + link.ToHTML() + ")")
}

type tableOpts struct {
	title       string
	description string
	width       string
}

func getID(in string) string {
	tmp := strings.ReplaceAll(in, "-", " ")
	tmp = strings.Title(tmp)
	tmp = strings.ReplaceAll(tmp, " ", "")
	return tmp
}

func getMainHeaderRow(name string) generichtml.HTMLTableRow {
	return generichtml.NewHTMLTableRowWithItems(map[string]string{}, []generichtml.HTMLItem{
		generichtml.HTMLTableHeaderRowItem{
			Text: name,
		},
		generichtml.HTMLTableHeaderRowItem{
			Text: "Trend",
		},
		generichtml.HTMLTableHeaderRowItem{
			Text: "Current 7 Days",
		},
		generichtml.HTMLTableHeaderRowItem{
			Text: "Previous 7 Days",
		},
	})
}

func getStepNameHeaderRow() generichtml.HTMLTableRow {
	return getMainHeaderRow("Step Name")
}

func getMultistageHeaderRow() generichtml.HTMLTableRow {
	return getMainHeaderRow("Multistage Job Name")
}

func initializeTable(opts tableOpts) generichtml.HTMLTable {
	table := generichtml.NewHTMLTable(map[string]string{
		"class": "table",
	})

	table.AddHeaderRow(generichtml.NewHTMLTableRowWithItems(map[string]string{}, []generichtml.HTMLItem{
		generichtml.HTMLTableHeaderRowItem{
			Params: map[string]string{
				"colspan": opts.width,
				"class":   "text-center",
			},
			HTMLItems: []generichtml.HTMLItem{
				generichtml.HTMLElement{
					Element: "a",
					Text:    opts.description,
					Params: map[string]string{
						"class": "text-dark",
						"id":    getID(opts.description),
						"href":  "#" + getID(opts.description),
					},
				},
				generichtml.HTMLElement{
					Element: "i",
					Params: map[string]string{
						"class": "fa fa-info-circle",
						"title": opts.title,
					},
				},
				generichtml.NewHTMLTextElement(" "),
			},
		},
	}))

	return table
}

func getArrowForTrend(t stepmetrics.Trend) string {
	return generichtml.GetArrowForTestResult(t.Current.TestResult, &t.Previous.TestResult)
}
