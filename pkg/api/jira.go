package api

import (
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
)

func GetJIRAIncidentsFromDB(dbClient *db.DB, start, end *time.Time) ([]apitype.CalendarEvent, error) {
	// Get JIRA Incidents for display in calendar
	incidents := make([]apitype.CalendarEvent, 0)
	res := dbClient.DB.Table("jira_incidents").Select(`
		start_time AS start,
		resolution_time AS end,
		key as jira,
		key || ': ' || summary AS title,
		'incident' AS phase,
		'TRUE' as all_day`).
		Where(`(start_time, resolution_time) OVERLAPS (?, ?)`, start, end).
		Scan(&incidents)

	return incidents, res.Error
}
