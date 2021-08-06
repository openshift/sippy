package generichtml_test

import (
	"net/url"
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
				Scheme: "https",
				Host:   "search.ci.openshift.org",
				Path:   "/test-1",
			}),
		},
		{
			name:  "test-2",
			count: "2",
			link: generichtml.NewHTMLLink("test-2", &url.URL{
				Scheme: "https",
				Host:   "search.ci.openshift.org",
				Path:   "/test-2",
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
    <td><a href="https://search.ci.openshift.org/test-1">test-1</a></td>
  </tr>
  <tr>
    <td>test-2</td>
    <td>2</td>
    <td><a href="https://search.ci.openshift.org/test-2">test-2</a></td>
  </tr>
</table>`

	result := table.ToHTML()
	if result != expected {
		t.Errorf("expected %s, got: %s", expected, result)
	}
}

func TestHTMLLink(t *testing.T) {
	htmlLink := generichtml.NewHTMLLinkWithParams(
		"Google",
		&url.URL{
			Scheme: "https",
			Host:   "www.google.com",
		},
		map[string]string{
			"class": "a-link",
		})

	results := htmlLink.ToHTML()

	expected := `<a class="a-link" href="https://www.google.com">Google</a>`

	if results != expected {
		t.Errorf("expected: %s, got: %s", expected, results)
	}
}
