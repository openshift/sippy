package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"gorm.io/gorm"
)

func PrintPullRequestsReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q, err := filterableDBResult(req, "releaseTag", apitype.SortDescending, releaseFilter(req, dbClient))
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	prs := make([]models.PullRequest, 0)
	q.Find(&prs)
	RespondWithJSON(http.StatusOK, w, prs)
}

func PrintReleaseJobRunsReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q, err := filterableDBResult(req, "releaseTag", apitype.SortDescending, releaseFilter(req, dbClient))
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	jobRuns := make([]models.JobRun, 0)
	q.Find(&jobRuns)
	RespondWithJSON(http.StatusOK, w, jobRuns)
}

func PrintReleasesReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q, err := filterableDBResult(req, "releaseTag", apitype.SortDescending, releaseFilter(req, dbClient))
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	releases := make([]models.ReleaseTag, 0)
	q.Find(&releases)
	RespondWithJSON(http.StatusOK, w, releases)
}

func PrintReleaseHealthReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	release := req.URL.Query().Get("release")
	if release == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Errorf(`"release" is required`),
		})
		return
	}

	results, err := models.GetLastAcceptedByArchitectureAndStream(dbClient.DB, release)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	RespondWithJSON(http.StatusOK, w, results)
}

func filterableDBResult(req *http.Request, defaultSortField string, defaultSort apitype.Sort, dbClient *gorm.DB) (*gorm.DB, error) {
	filter := &Filter{}
	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			return nil, fmt.Errorf("could not marshal filter: %w", err)
		}
	}
	q := filter.ToSQL(dbClient)
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 {
		q = q.Limit(limit)
	}

	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))
	if sortField == "" {
		sortField = defaultSortField
	}
	if sort == "" {
		sort = defaultSort
	}
	q.Order(fmt.Sprintf("%q %s", sortField, sort))

	return q, nil
}

func releaseFilter(req *http.Request, dbClient *db.DB) *gorm.DB {
	releaseFilter := req.URL.Query().Get("release")
	if releaseFilter != "" {
		return dbClient.DB.Where("release = ?", releaseFilter)
	}

	return dbClient.DB
}
