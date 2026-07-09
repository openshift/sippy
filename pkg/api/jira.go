package api

import (
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
)

func GetJIRAIncidentsFromDB(dbClient *db.DB, start, end *time.Time) ([]apitype.CalendarEvent, error) {
	// Rounding to start of next day because of https://github.com/fullcalendar/fullcalendar/issues/7413
	now := time.Now().UTC()
	startOfDay := now.Truncate(24 * time.Hour)
	startOfNextDay := startOfDay.Add(24 * time.Hour)

	// Get JIRA Incidents for display in calendar
	incidents := make([]apitype.CalendarEvent, 0)
	res := dbClient.DB.Table("jira_incidents").Select(`
		start_time AS start,
		COALESCE(DATE_TRUNC('day', resolution_time) + interval '1 day', ?) AS end,
		key as jira,
		key || ': ' || summary AS title,
		'incident' AS phase,
		'TRUE' as all_day`, startOfNextDay,
	).
		Where(`(start_time, COALESCE(resolution_time, ?)) OVERLAPS (?, ?)`, startOfNextDay, start, end).
		Where(`(start_time, resolution_time) OVERLAPS (?, ?)`, start, end).
		Scan(&incidents)

	return incidents, res.Error
}
