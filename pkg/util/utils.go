package util

import (
	"regexp"
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
)

type FailureGroupStats struct {
	Count      int
	CountPrev  int
	Median     int
	MedianPrev int
	Avg        int
	AvgPrev    int
}

// ComputeFailureGroupStats computes count, median, and average number of failuregroups
// returns FailureGroupStats containing count, countPrev, median, medianPrev, avg, avgPrev

func RelevantJob(jobName, status string, filter *regexp.Regexp) bool {
	if filter != nil && !filter.MatchString(jobName) {
		return false
	}
	return true
}

func IsActiveBug(bug bugsv1.Bug) bool {
	switch bug.Status {
	case "VERIFIED", "RELEASE_PENDING", "CLOSED":
		return false
	default:
		return true
	}
}

func StrSliceContains(strSlice []string, elem string) bool {
	for _, s := range strSlice {
		if s == elem {
			return true
		}
	}
	return false
}

// PeriodToDates takes a period name such as twoDay or default, and
// converts to start, boundary, and end times.
func PeriodToDates(period string, timeNow time.Time) (start, boundary, end time.Time) {
	if period == "twoDay" {
		start = timeNow.Add(-9 * 24 * time.Hour)
		boundary = timeNow.Add(-2 * 24 * time.Hour)
	} else {
		start = timeNow.Add(-14 * 24 * time.Hour)
		boundary = timeNow.Add(-7 * 24 * time.Hour)
	}
	end = timeNow

	return start, boundary, end
}

func GetTimeNow(pinnedTime *time.Time) time.Time {
	if pinnedTime == nil {
		return time.Now()
	}

	return *pinnedTime
}
