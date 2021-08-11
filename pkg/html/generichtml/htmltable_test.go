package generichtml_test

import (
	"net/url"
	"regexp"
	"testing"

	"github.com/openshift/sippy/pkg/html/generichtml"
)

func TestHTMLTable(t *testing.T) {
	table := generichtml.NewHTMLTable(map[string]string{
		"class": "table",
	})

	headerRow := generichtml.NewHTMLTableRow(map[string]string{
		"class": "header",
	})

	headerRow.AddItems([]generichtml.HTMLItem{
		generichtml.HTMLTableHeaderRowItem{
			Text: "Name",
		},
		generichtml.HTMLTableHeaderRowItem{
			Text: "Count",
		},
		generichtml.HTMLTableHeaderRowItem{
			Text: "Link",
		},
	})

	table.AddHeaderRow(headerRow)

	items := []struct {
		name  string
		count string
		link  generichtml.HTMLItem
	}{
		{
			name:  "test-1",
			count: "1",
			link: generichtml.NewHTMLLink("test-1", &url.URL{
				Scheme:   "https",
				Host:     "search.ci.openshift.org",
				RawQuery: getURLValues().Encode(),
				Path:     "/test-1",
			}),
		},
		{
			name:  "test-2",
			count: "2",
			link: generichtml.NewHTMLLink("test-2", &url.URL{
				Scheme:   "https",
				Host:     "search.ci.openshift.org",
				RawQuery: getURLValues().Encode(),
				Path:     "/test-2",
			}),
		},
	}

	for _, item := range items {
		row := generichtml.NewHTMLTableRow(map[string]string{})

		row.AddItems([]generichtml.HTMLItem{
			generichtml.HTMLTableRowItem{
				Text: item.name,
			},
			generichtml.HTMLTableRowItem{
				Text: item.count,
			},
			generichtml.HTMLTableRowItem{
				HTMLItems: []generichtml.HTMLItem{
					item.link,
				},
			},
		})

		table.AddRow(row)
	}

	expected := `<table class="table">
  <tr class="header">
    <th>Name</th>
    <th>Count</th>
    <th>Link</th>
  </tr>
  <tr>
    <td>test-1</td>
    <td>1</td>
    <td><a href="https://search.ci.openshift.org/test-1?context=1&groupBy=job&maxAge=168h&maxBytes=20971520&maxMatches=5&name=4.9&search=operator%5C.Run+multistage+test+e2e-aws+-+e2e-aws-openshift-e2e-test+container+test&type=bug%2Bjunit">test-1</a></td>
  </tr>
  <tr>
    <td>test-2</td>
    <td>2</td>
    <td><a href="https://search.ci.openshift.org/test-2?context=1&groupBy=job&maxAge=168h&maxBytes=20971520&maxMatches=5&name=4.9&search=operator%5C.Run+multistage+test+e2e-aws+-+e2e-aws-openshift-e2e-test+container+test&type=bug%2Bjunit">test-2</a></td>
  </tr>
</table>`

	result := table.ToHTML()
	if result != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
	}
}

func TestHTMLLink(t *testing.T) {
	htmlLink := generichtml.NewHTMLLinkWithParams(
		"CI Search",
		&url.URL{
			Scheme:   "https",
			Host:     "search.ci.openshift.org",
			RawQuery: getURLValues().Encode(),
			Path:     "search",
		},
		map[string]string{
			"class": "a-link",
		})

	results := htmlLink.ToHTML()

	expected := `<a class="a-link" href="https://search.ci.openshift.org/search?context=1&groupBy=job&maxAge=168h&maxBytes=20971520&maxMatches=5&name=4.9&search=operator%5C.Run+multistage+test+e2e-aws+-+e2e-aws-openshift-e2e-test+container+test&type=bug%2Bjunit">CI Search</a>`

	if results != expected {
		t.Errorf("expected: %s, got: %s", expected, results)
	}
}

func getURLValues() url.Values {
	urlValues := url.Values{}

	urlValues.Add("maxAge", "168h")
	urlValues.Add("context", "1")
	urlValues.Add("type", "bug+junit")
	urlValues.Add("name", "4.9")
	urlValues.Add("maxMatches", "5")
	urlValues.Add("maxBytes", "20971520")
	urlValues.Add("groupBy", "job")
	urlValues.Add("search", regexp.QuoteMeta("operator.Run multistage test e2e-aws - e2e-aws-openshift-e2e-test container test"))

	return urlValues
}
