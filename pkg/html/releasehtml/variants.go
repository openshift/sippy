package releasehtml

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
)

var variantStart = template.Must(template.New("variants").Parse(`
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
`))

// PrintVariantsReport shows an aggregated listing of all jobs for a particular variant.
func PrintVariantsReport(w http.ResponseWriter, release, variant string, currentWeek, previousWeek *v1.VariantResults, timestamp time.Time) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")

	var s string
	for _, currJobResult := range currentWeek.JobResults {
		prevJobResult := util.FindJobResultForJobName(currJobResult.Name, previousWeek.JobResults)
		jobHTML := generichtml.NewJobResultRendererFromJobResult("by-variant", currJobResult, release).
			WithMaxTestResultsToShow(10).
			WithPreviousJobResult(prevJobResult).
			ToHTML()

		s += jobHTML
	}

	fmt.Fprintf(w, generichtml.HTMLPageStart, "Job Results for Variant "+variant)
	if err := variantStart.Execute(w, map[string]interface{}{
		"Variant": variant,
		"Release": release,
	}); err != nil {
		log.Error(err)
	}

	fmt.Fprint(w, s+"</table>")
	fmt.Fprintf(w, generichtml.HTMLPageEnd, timestamp.Format("Jan 2 15:04 2006 MST"))
}
