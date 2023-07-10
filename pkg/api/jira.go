package api

import (
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
)

func GetJIRAIncidentsFromDB(dbClient *db.DB) ([]apitype.CalendarEvent, error) {
	// Get JIRA Incidents for display in calendar
	var incidents []apitype.CalendarEvent
	res := dbClient.DB.Table("jira_incidents").Select(`
		start_time AS start,
		resolution_time AS end,
		key as jira,
		key || ': ' || summary AS title,
		'incident' AS phase,
		'TRUE' as all_day`).Scan(&incidents)

	return incidents, res.Error
}
