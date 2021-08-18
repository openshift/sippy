package stepmetricshtml

import (
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/html/generichtml"
)

type tableOpts struct {
	pageTitle   string
	title       string
	description string
	width       string
	headerRows  []generichtml.HTMLTableRow
	rows        []tableRow
}

func (t tableOpts) toHTML() string {
	return t.toTable().ToHTML()
}

func (t tableOpts) toTable() generichtml.HTMLTable {
	table := generichtml.NewHTMLTable(map[string]string{
		"class": "table",
	})

	defaultHeaderRow := generichtml.NewHTMLTableRowWithItems(map[string]string{}, []generichtml.HTMLItem{
		generichtml.HTMLTableHeaderRowItem{
			Params: map[string]string{
				"colspan": t.width,
				"class":   "text-center",
			},
			HTMLItems: []generichtml.HTMLItem{
				generichtml.HTMLElement{
					Element: "a",
					Text:    t.description,
					Params: map[string]string{
						"class": "text-dark",
						"id":    getID(t.description),
						"href":  "#" + getID(t.description),
					},
				},
				generichtml.HTMLElement{
					Element: "i",
					Params: map[string]string{
						"class": "fa fa-info-circle",
						"title": t.title,
					},
				},
				generichtml.NewHTMLTextElement(" "),
			},
		},
	})

	allHeaderRows := append([]generichtml.HTMLTableRow{
		defaultHeaderRow,
	}, t.headerRows...)

	for _, headerRow := range allHeaderRows {
		table.AddHeaderRow(headerRow)
	}

	for _, row := range t.rows {
		table.AddRow(row.toHTMLTableRow())
	}

	return table
}

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
		nameItems = append(nameItems, getEnclosedHTMLLink(t.sippyURL))
	}

	if t.ciSearchURL != nil {
		nameItems = append(nameItems, getEnclosedHTMLLink(t.ciSearchURL))
	}

	if t.stepRegistryURL != nil {
		nameItems = append(nameItems, getEnclosedHTMLLink(t.stepRegistryURL))
	}

	row.AddItems([]generichtml.HTMLItem{
		generichtml.HTMLTableRowItem{
			HTMLItems: generichtml.SpaceHTMLItems(nameItems),
		},
		generichtml.HTMLTableRowItem{
			Text: getArrowForTrend(t.trend),
		},
		generichtml.HTMLTableRowItem{
			Text: fmt.Sprintf("%0.2f%% (%d runs)", t.trend.Current.PassPercentage, t.trend.Current.Runs),
		},
		generichtml.HTMLTableRowItem{
			Text: fmt.Sprintf("%0.2f%% (%d runs)", t.trend.Current.PassPercentage, t.trend.Previous.Runs),
		},
	})

	return row
}

func getEnclosedHTMLLink(linkURL URLGenerator) generichtml.HTMLItem {
	return generichtml.NewHTMLTextElement("(" + linkURL.ToHTML() + ")")
}

func getID(in string) string {
	tmp := strings.ReplaceAll(in, "-", " ")
	tmp = strings.Title(tmp)
	tmp = strings.ReplaceAll(tmp, " ", "")
	tmp = strings.ReplaceAll(tmp, ".", "")
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

func getArrowForTrend(t stepmetrics.Trend) string {
	return generichtml.GetArrowForTestResult(t.Current.TestResult, &t.Previous.TestResult)
}
