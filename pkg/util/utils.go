package util

import (
	"fmt"
	"math"
	gourl "net/url"
	"time"
)

type FailureGroupStats struct {
	Count      int
	CountPrev  int
	Median     int
	MedianPrev int
	Avg        int
	AvgPrev    int
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
func PeriodToDates(period string, reportEnd time.Time) (start, boundary, end time.Time) {
	if period == "twoDay" {
		start = reportEnd.Add(-9 * 24 * time.Hour)
		boundary = reportEnd.Add(-2 * 24 * time.Hour)
	} else {
		start = reportEnd.Add(-14 * 24 * time.Hour)
		boundary = reportEnd.Add(-7 * 24 * time.Hour)
	}
	end = reportEnd

	return start, boundary, end
}

func GetReportEnd(pinnedTime *time.Time) time.Time {
	if pinnedTime == nil {
		return time.Now()
	}

	return *pinnedTime
}

func IsNeverStable(variants []string) bool {
	for _, variant := range variants {
		if variant == "never-stable" {
			return true
		}
	}

	return false
}

// ConvertNaNToZero prevents attempts to marshal the NaN zero-value of a float64 in go by converting to 0.
func ConvertNaNToZero(f float64) float64 {
	if math.IsNaN(f) {
		return 0.0
	}

	return f
}

func URLForJob(dashboard, jobName string) *gourl.URL {
	url := &gourl.URL{
		Scheme: "https",
		Host:   "testgrid.k8s.io",
		Path:   fmt.Sprintf("/%s", gourl.PathEscape(dashboard)),
	}
	// this is a non-standard fragment honored by test-grid
	url.Fragment = gourl.PathEscape(jobName)

	return url
}
