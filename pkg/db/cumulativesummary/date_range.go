package cumulativesummary

import (
	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
)

const maxAutoFillDays = 14

// resolveStartDate determines the start date for an incremental refresh.
// It picks the earliest of earliestChanged and maxExistingDate+1, then
// clamps to today-maxAutoFillDays to prevent unbounded backfills during
// normal refresh cycles.
func resolveStartDate(earliestChanged civil.Date, maxExistingDate *civil.Date, today civil.Date) civil.Date {
	startDate := earliestChanged
	if maxExistingDate != nil {
		nextDate := maxExistingDate.AddDays(1)
		if nextDate.Before(startDate) {
			startDate = nextDate
		}
	}

	floor := today.AddDays(-maxAutoFillDays)
	if startDate.Before(floor) {
		log.WithFields(log.Fields{
			"desiredStart": startDate,
			"clampedStart": floor,
		}).Warn("cumulative summaries need data older than the auto-fill limit, use backfill command to fill")
		startDate = floor
	}

	return startDate
}
