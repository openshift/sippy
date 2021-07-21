package releasehtml

import (
	"fmt"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/util"
	"html/template"
	"net/http"
	"time"
)

var variantsTemplate = template.Must(template.New("variants").Parse(`
<div align="center">
	<h1>Results for {{.Variant}} on {{.Release}}</h1>
</div>

<table class="table">
<tr>
<th colspan=5 class="text-center">
<a class="text-dark"  id="PassRateByVariantJob" href="#PassRateByVariantJob">Pass Rate By Variant Job</a>
<i class="fa fa-info-circle" title="Aggregation of all job runs for a given variant, sorted by passing rate percentage."></i>
</th>
</tr>
<tr>
<th>Job</th><th></th><th>Latest 7 days</th><th></th><th>Previous 7 days</th>
</tr>
{{.Results}}
</table>
`))

type templateData struct {
	Release string
	Variant string
	Results template.HTML
}


// PrintVariantsReport shows an aggregated listing of all jobs for a particular variant.
func PrintVariantsReport(w http.ResponseWriter, release, variant string, currentWeek, previousWeek v1.VariantResults, timestamp time.Time) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")

	results := template.HTML(generichtml.NewJobAggregationResultRendererFromVariantResults("by-job", currentWeek, release).
		WithMaxTestResultsToShow(10).
		WithMaxJobResultsToShow(0).
		WithPreviousVariantResults(util.FindVariantResultsForName(currentWeek.VariantName, []v1.VariantResults{previousWeek})).
		ToHTML())

	fmt.Fprintf(w, generichtml.HTMLPageStart, "Job Results for Variant " + variant)
	variantsTemplate.Execute(w, map[string]interface{}{
		"Variant": variant,
		"Release": release,
		"Results": results,
	})
	fmt.Fprintf(w, generichtml.HTMLPageEnd, timestamp.Format("Jan 2 15:04 2006 MST"))
}
