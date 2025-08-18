package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openshift/sippy/pkg/db"
)

// PrintAutocompleteFromDB returns autocomplete results for a particular field,
// such as test or job names. It optionally takes a release and search query filter.
func PrintAutocompleteFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB) {
	result := make([]string, 0)
	vars := mux.Vars(req)
	field := vars["field"]
	search := req.URL.Query().Get("search")
	release := req.URL.Query().Get("release")

	q := dbc.DB

	switch field {
	case "variants": //nolint:goconst
		q = q.Table("prow_jobs").
			Select("DISTINCT(unnest(variants)) as name").
			Order("name")
	case "tests":
		q = q.Table("tests").
			Select("name").
			Order("name")
	case "jobs":
		q = q.Table("prow_jobs").
			Select("name").
			Order("name")
	case "orgs":
		q = q.Table("prow_pull_requests").
			Select("DISTINCT(org) as name").
			Order("name")
	case "repos":
		q = q.Table("prow_pull_requests").
			Select("DISTINCT(repo) as name").
			Order("name")
	case "authors":
		q = q.Table("prow_pull_requests").
			Select("DISTINCT(author) as name").
			Order("name")
	case "cluster":
		q = q.Table("prow_job_runs").
			Select("DISTINCT(cluster) as name").
			Where("cluster IS NOT NULL").
			Order("name")
	case "suite_name":
		q = q.Table("suites").
			Select("name").
			Order("name")
	case "jira_component":
		q = q.Table("jira_components").
			Select("name").
			Order("name")
	default:
		RespondWithJSON(404, w, map[string]string{"message": "Autocomplete field not found."})
	}

	if release != "" {
		q = q.Where("release = ?", release)
	}

	if search != "" {
		sq := dbc.DB.Table("(?) as q", q)
		q = sq.Where("name ILIKE ?", fmt.Sprintf("%%%s%%", search))
	}

	q = q.Limit(50).Scan(&result)
	if q.Error != nil {
		RespondWithJSON(503, w, map[string]string{"message": q.Error.Error()})
		return
	}

	RespondWithJSON(200, w, result)
}
