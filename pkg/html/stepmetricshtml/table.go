package stepmetricshtml

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/util/sets"
)

type StepMetricsHTMLTable struct {
	StepMetricsHTTPQuery
	api StepMetricsAPI
}

func NewStepMetricsHTMLTable(q StepMetricsHTTPQuery) StepMetricsHTMLTable {
	return StepMetricsHTMLTable{
		StepMetricsHTTPQuery: q,
		api:                  NewStepMetricsAPI(q),
	}
}

func (s StepMetricsHTMLTable) Render(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, generichtml.HTMLPageStart, fmt.Sprintf("Release %s Step Metrics Dashboard", s.Release))
	fmt.Fprint(w, s.getTable().ToHTML())
	fmt.Fprintf(w, generichtml.HTMLPageEnd, s.Current.Timestamp.Format("Jan 2 15:04 2006 MST"))
}

func (s StepMetricsHTMLTable) getTable() generichtml.HTMLTable {
	if s.isMultistageQuery() {
		return s.ByMultistageName()
	}

	if s.isStepQuery() {
		return s.ByStageName()
	}

	return generichtml.NewHTMLTable(map[string]string{})
}

func (s StepMetricsHTMLTable) ByMultistageName() generichtml.HTMLTable {
	if s.MultistageJobName == All {
		return s.allMultistageJobNames()
	}

	return s.stepNamesForMultistageJobName(s.MultistageJobName)
}

func (s StepMetricsHTMLTable) stepNamesForMultistageJobName(multistageJobName string) generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		title:       "All Step Names for Multistage Job " + multistageJobName,
		description: "(stepNamesForMultistageJobNames) All Step Names For Multistage Job " + multistageJobName,
		width:       "4",
	})

	table.AddHeaderRow(getStepNameHeaderRow())

	multistageDetails := s.api.GetMultistage(multistageJobName)

	stepNames := sets.StringKeySet(multistageDetails.StepDetails).List()

	for _, stepName := range stepNames {
		stepDetail := multistageDetails.StepDetails[stepName]
		table.AddRow(getRowForStepDetail(stepDetail, s.Release))
	}

	return table
}

func (s StepMetricsHTMLTable) allMultistageJobNames() generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		width:       "4",
		title:       "All Multistage Job Names",
		description: "(allMultistageJobNames) All Multistage Jobs",
	})

	table.AddHeaderRow(getMultistageHeaderRow())

	allMultistages := s.api.AllMultistages()
	sort.Slice(allMultistages, func(i, j int) bool {
		return allMultistages[i].Name < allMultistages[j].Name
	})

	for _, multistageDetail := range allMultistages {
		sippyURL := &SippyURL{
			Release:           s.Release,
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
		title:       "Frequency of passes / failures for step registry items for " + s.StepName,
		description: "(allStageNames) Step Metrics For " + s.StepName,
		width:       "4",
	})

	table.AddHeaderRow(getStepNameHeaderRow())

	allStages := s.api.AllStages()
	sort.Slice(allStages, func(i, j int) bool {
		return allStages[i].Name < allStages[j].Name
	})

	for _, stepDetails := range allStages {
		sippyURL := SippyURL{
			Release:  s.Release,
			StepName: stepDetails.Name,
		}

		ciSearchURL := CISearchURL{
			Release: s.Release,
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

func (s StepMetricsHTMLTable) forStageName(stageName string) generichtml.HTMLTable {
	table := initializeTable(tableOpts{
		title:       "Frequency of passes / failures for " + stageName + " by multistage job name",
		description: "(forStageName) Step Metrics For " + stageName + " By Multistage Job Name",
		width:       "4",
	})

	table.AddHeaderRow(getMultistageHeaderRow())

	stageResult := s.api.GetStage(stageName)
	multistageNames := sets.StringKeySet(stageResult.ByMultistage)

	for _, multistageName := range multistageNames.List() {
		multistageResult := s.api.GetMultistage(multistageName)

		sippyURL := &SippyURL{
			Release:           s.Release,
			MultistageJobName: multistageName,
		}

		stepRegistryURL := StepRegistryURL{
			Search: multistageResult.Name,
		}

		ciSearchURL := CISearchURL{
			Release: s.Release,
			Search:  multistageResult.StepDetails[stageName].Current.OriginalTestName,
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

func (s StepMetricsHTMLTable) ByStageName() generichtml.HTMLTable {
	if s.StepName == All {
		return s.allStageNames()
	}

	return s.forStageName(s.StepName)
}

func (s StepMetricsHTMLTable) StageNameDetail() generichtml.HTMLTable {
	return generichtml.HTMLTable{}
}

func (s StepMetricsHTMLTable) MultistageDetail() generichtml.HTMLTable {
	return generichtml.HTMLTable{}
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
