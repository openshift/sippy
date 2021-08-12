package stepmetricshtml

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/util/sets"
)

type StepMetricsHTMLTable struct {
	api       StepMetricsAPI
	release   string
	timestamp time.Time
}

func NewStepMetricsHTMLTable(curr, prev sippyprocessingv1.TestReport) StepMetricsHTMLTable {
	return StepMetricsHTMLTable{
		api:       NewStepMetricsAPI(curr, prev),
		release:   curr.Release,
		timestamp: curr.Timestamp,
	}
}

func (s StepMetricsHTMLTable) Render(w http.ResponseWriter, req Request) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, generichtml.HTMLPageStart, fmt.Sprintf("Release %s Step Metrics Dashboard", s.release))
	fmt.Fprint(w, s.getTable(req).ToHTML())
	fmt.Fprintf(w, generichtml.HTMLPageEnd, s.timestamp.Format("Jan 2 15:04 2006 MST"))
}

func (s StepMetricsHTMLTable) getTable(req Request) generichtml.HTMLTable {
	if req.MultistageJobName != "" {
		return s.ByMultistageName(req)
	}

	if req.StepName != "" {
		return s.ByStageName(req)
	}

	return generichtml.NewHTMLTable(map[string]string{})
}

func (s StepMetricsHTMLTable) ByMultistageName(req Request) generichtml.HTMLTable {
	if req.MultistageJobName == All {
		return s.allMultistageJobNames()
	}

	return s.stepNamesForMultistageJobName(req)
}

func (s StepMetricsHTMLTable) stepNamesForMultistageJobName(req Request) generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		title:       "All Step Names for Multistage Job " + req.MultistageJobName,
		description: "All Step Names For Multistage Job " + req.MultistageJobName,
		width:       "4",
	})

	table.AddHeaderRow(getStepNameHeaderRow())

	multistageDetails := s.api.GetMultistage(req)

	stepNames := sets.StringKeySet(multistageDetails.StepDetails).List()

	for _, stepName := range stepNames {
		stepDetail := multistageDetails.StepDetails[stepName]
		table.AddRow(getRowForStepDetail(stepDetail, s.release))
	}

	return table
}

func (s StepMetricsHTMLTable) allMultistageJobNames() generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		width:       "4",
		title:       "All Multistage Job Names",
		description: "All Multistage Jobs",
	})

	table.AddHeaderRow(getMultistageHeaderRow())

	allMultistages := s.api.AllMultistages()
	sort.Slice(allMultistages, func(i, j int) bool {
		return allMultistages[i].Name < allMultistages[j].Name
	})

	for _, multistageDetail := range allMultistages {
		sippyURL := &SippyURL{
			Release:           s.release,
			MultistageJobName: multistageDetail.Name,
		}

		stepRegistryURL := StepRegistryURL{
			Search: multistageDetail.Name,
		}

		row := generichtml.NewHTMLTableRow(map[string]string{})
		row.AddItems([]generichtml.HTMLItem{
			generichtml.HTMLTableRowItem{
				HTMLItems: generichtml.SpaceHTMLItems([]generichtml.HTMLItem{
					generichtml.NewHTMLTextElement(multistageDetail.Name),
					generichtml.NewHTMLLink("Step Registry", stepRegistryURL.URL()),
					generichtml.NewHTMLLink("Detail", sippyURL.URL()),
				}),
			},
			generichtml.HTMLTableRowItem{
				Text: multistageDetail.getArrow(),
			},
			generichtml.HTMLTableRowItem{
				Text: getStageResultDetail(multistageDetail.Current),
			},
			generichtml.HTMLTableRowItem{
				Text: getStageResultDetail(multistageDetail.Previous),
			},
		})
		table.AddRow(row)
	}

	return table
}

func (s StepMetricsHTMLTable) allStageNames() generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		title:       "Frequency of passes / failures for step registry items for all steps",
		description: "Step Metrics For All Steps",
		width:       "4",
	})

	table.AddHeaderRow(getStepNameHeaderRow())

	allStages := s.api.AllStages()
	sort.Slice(allStages, func(i, j int) bool {
		return allStages[i].Name < allStages[j].Name
	})

	for _, stepDetails := range allStages {
		sippyURL := SippyURL{
			Release:  s.release,
			StepName: stepDetails.Name,
		}

		ciSearchURL := CISearchURL{
			Release: s.release,
			Search:  stepDetails.Current.OriginalTestName,
		}

		stepRegistryURL := StepRegistryURL{
			Reference: stepDetails.Name,
		}

		row := generichtml.NewHTMLTableRow(map[string]string{})
		row.AddItems([]generichtml.HTMLItem{
			generichtml.HTMLTableRowItem{
				HTMLItems: generichtml.SpaceHTMLItems([]generichtml.HTMLItem{
					htmlTextItem(stepDetails.Name + " "),
					getEnclosedHTMLLink(generichtml.NewHTMLLink("Detail", sippyURL.URL())),
					getEnclosedHTMLLink(generichtml.NewHTMLLink("CI Search", ciSearchURL.URL())),
					getEnclosedHTMLLink(generichtml.NewHTMLLink("Step Registry", stepRegistryURL.URL())),
				}),
			},
			generichtml.HTMLTableRowItem{
				Text: stepDetails.getArrow(),
			},
			generichtml.HTMLTableRowItem{
				Text: getStageResultDetail(stepDetails.Current),
			},
			generichtml.HTMLTableRowItem{
				Text: getStageResultDetail(stepDetails.Previous),
			},
		})

		table.AddRow(row)
	}

	return table
}

func (s StepMetricsHTMLTable) forStageName(req Request) generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		title:       "Frequency of passes / failures for " + req.StepName + " by multistage job name",
		description: "Step Metrics For " + req.StepName + " By Multistage Job Name",
		width:       "4",
	})

	table.AddHeaderRow(getMultistageHeaderRow())

	stageResult := s.api.GetStage(req)
	multistageNames := sets.StringKeySet(stageResult.ByMultistage)

	for _, multistageName := range multistageNames.List() {
		multistageResult := s.api.GetMultistage(Request{
			MultistageJobName: multistageName,
		})

		sippyURL := &SippyURL{
			Release:           s.release,
			MultistageJobName: multistageName,
		}

		stepRegistryURL := StepRegistryURL{
			Search: multistageResult.Name,
		}

		ciSearchURL := CISearchURL{
			Release: s.api.current.Release,
			Search:  multistageResult.StepDetails[stageResult.Name].Current.OriginalTestName,
		}

		row := generichtml.NewHTMLTableRow(map[string]string{})
		row.AddItems([]generichtml.HTMLItem{
			generichtml.HTMLTableRowItem{
				HTMLItems: generichtml.SpaceHTMLItems([]generichtml.HTMLItem{
					generichtml.NewHTMLTextElement(multistageName),
					generichtml.NewHTMLLink("Step Registry", stepRegistryURL.URL()),
					generichtml.NewHTMLLink("CI Search", ciSearchURL.URL()),
					generichtml.NewHTMLLink("Detail", sippyURL.URL()),
				}),
			},
			generichtml.HTMLTableRowItem{
				Text: multistageResult.getArrow(),
			},
			generichtml.HTMLTableRowItem{
				Text: getStageResultDetail(multistageResult.Current),
			},
			generichtml.HTMLTableRowItem{
				Text: getStageResultDetail(multistageResult.Previous),
			},
		})

		table.AddRow(row)
	}

	return table
}

func (s StepMetricsHTMLTable) ByStageName(req Request) generichtml.HTMLTable {
	if req.StepName == All {
		return s.allStageNames()
	}

	return s.forStageName(req)
}

func getStageResultDetail(stageResult sippyprocessingv1.StageResult) string {
	runs := stageResult.Successes + stageResult.Failures + stageResult.Flakes
	return fmt.Sprintf("%0.2f (Runs: %d)", stageResult.PassPercentage, runs)
}

type htmlTextItem string

func (h htmlTextItem) ToHTML() string {
	return string(h)
}

func getEnclosedHTMLLink(link generichtml.HTMLItem) generichtml.HTMLItem {
	return htmlTextItem("(" + link.ToHTML() + ")")
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

func getRowForStepDetail(stepDetail StepDetail, release string) generichtml.HTMLTableRow {
	sippyURL := SippyURL{
		Release:  release,
		StepName: stepDetail.Name,
	}

	ciSearchURL := CISearchURL{
		Release: release,
		Search:  stepDetail.Current.OriginalTestName,
	}

	stepRegistryURL := StepRegistryURL{
		Reference: stepDetail.Name,
	}

	row := generichtml.NewHTMLTableRow(map[string]string{})
	row.AddItems([]generichtml.HTMLItem{
		generichtml.HTMLTableRowItem{
			HTMLItems: generichtml.SpaceHTMLItems([]generichtml.HTMLItem{
				htmlTextItem(stepDetail.Name + " "),
				getEnclosedHTMLLink(generichtml.NewHTMLLink("Detail", sippyURL.URL())),
				getEnclosedHTMLLink(generichtml.NewHTMLLink("CI Search", ciSearchURL.URL())),
				getEnclosedHTMLLink(generichtml.NewHTMLLink("Step Registry", stepRegistryURL.URL())),
			}),
		},
		generichtml.HTMLTableRowItem{
			Text: stepDetail.getArrow(),
		},
		generichtml.HTMLTableRowItem{
			Text: getStageResultDetail(stepDetail.Current),
		},
		generichtml.HTMLTableRowItem{
			Text: getStageResultDetail(stepDetail.Previous),
		},
	})

	return row
}
