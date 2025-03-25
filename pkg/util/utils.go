package util

import (
	"context"
	"fmt"
	"math"
	gourl "net/url"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
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

func DatePtr(year int, month time.Month, day, hour, min, sec, nsec int, loc *time.Location) *time.Time {
	d := time.Date(year, month, day, hour, min, sec, nsec, loc)
	return &d
}

// releaseRelativeRE is a custom format we allow for times relative to now, or a releases ga date
// (i.e. now-7d, ga-30d, ga, etc
var releaseRelativeRE = regexp.MustCompile(`^(now|ga|end)(?:-([0-9]+)([d]))?$`)

// ParseCRReleaseTime parses the time for component readiness. The string can be a fully qualified
// RFC8339 string, or a custom "relative to now/ga" string we support for views. (examples: now, now-7d,
// ga, ga-30d, end-90d)
//
// It then adjusts the time based on a rounding factor if queried for "today". This is essentially a cache window used to keep
// results consistent as various sub-queries are run for components/features. If the round factor of
// 4h is used and a timeStr is provided which matches today, the timeStr will be rounded down to the nearest
// even 4h. i.e. 04:00, 08:00, 12:00, etc.
//
// isStart indicates if a relative time string should round down (base/sample start time), or up (base/sample end time).
// i.e. isStart=true, we would round down to 00:00:00 for the resulting times date.
// For isStart=false we would round up to 23:59:59.
//
// endTime must be specified if your timeStr uses the end directive. (end-90d) Otherwise it is not required or used.
func ParseCRReleaseTime(allReleases []v1.Release, release, timeStr string, isStart bool, endTime *time.Time, crTimeRoundingFactor time.Duration) (time.Time, error) {

	var relTime time.Time

	gaDateMap := map[string]time.Time{}
	for _, r := range allReleases {
		if r.GADate != nil {
			gaDateMap[r.Release] = *r.GADate
		}
	}

	// Check if the time parses as our custom format for times relative to now/ga:
	matches := releaseRelativeRE.FindStringSubmatch(timeStr)
	if matches != nil {
		switch matches[1] {
		case "now":
			relTime = time.Now()
		case "ga":
			var ok bool
			relTime, ok = gaDateMap[release]
			if !ok {
				return time.Time{}, fmt.Errorf("unable to find ga date for %s", release)
			}
		case "end":
			if endTime == nil {
				return time.Time{}, fmt.Errorf("caller provided no end time for %s", timeStr)
			}
			if !isStart {
				return time.Time{}, fmt.Errorf("cannot calculate a relative end date for an end date: %s", timeStr)
			}
			relTime = *endTime
		}
		return AdjustReleaseTime(relTime, isStart, matches[2], crTimeRoundingFactor), nil
	}

	// Parse as a fully qualified timestamp:
	var err error
	relTime, err = time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return relTime, err
	}

	// Apply the rounding factor:
	now := time.Now().UTC()
	if crTimeRoundingFactor > 0 && now.Format("2006-01-02") == relTime.Format("2006-01-02") {
		relTime = now.Truncate(crTimeRoundingFactor)
	}
	return relTime, nil
}

func AdjustReleaseTime(relTime time.Time, isStart bool, daysAdjustment string, crTimeRoundingFactor time.Duration) time.Time {
	// adjust by number of days:
	adjustDays, _ := strconv.ParseInt(daysAdjustment, 10, 64)
	adjustDur := time.Duration(adjustDays) * 24 * time.Hour
	relTime = relTime.Add(-adjustDur)
	// Now round to start/end of day as appropriate:
	if isStart {
		relTime = time.Date(relTime.Year(), relTime.Month(), relTime.Day(), 0, 0, 0, 0, time.UTC)

	} else {
		// Apply the rounding factor if using today:
		now := time.Now().UTC()
		if crTimeRoundingFactor > 0 && now.Format("2006-01-02") == relTime.Format("2006-01-02") {
			relTime = now.Truncate(crTimeRoundingFactor)
		} else {
			// otherwise round up to end of day
			relTime = time.Date(relTime.Year(), relTime.Month(), relTime.Day(), 23, 59, 59, 0, time.UTC)
		}
	}
	return relTime
}

/*
	  Enable functional tests requiring a live database and/or GCS bucket with known data to run,
	  but we do not want them trying to run during CI, so skip tests whose required environment variables are not set.
	  Don't risk checking in credentials with code; supply them with environment variables:
		TEST_DB_LOG_LEVEL: "silent" or "info" or "warn" or "error" - the log level for gorm database methods
		TEST_SIPPY_DATABASE_DSN: the DSN for the sippy postgres database e.g. postgresql://sippyro:...@sippy-postgresql...amazonaws.com/sippy_openshift
		TEST_GCS_CREDS_PATH: the path to a local GCS credentials file, e.g. /home/$USER/git/sippy/openshift-sippy-ro.creds.json
*/
const GcsBucketRoot = "test-platform-results"

func GetDbHandle(t *testing.T) *db.DB {
	dbLogLevel := os.Getenv("TEST_DB_LOG_LEVEL") // e.g. "info" or "silent"
	if dbLogLevel == "" {
		dbLogLevel = "silent"
	}
	gormLogLevel, err := db.ParseGormLogLevel(dbLogLevel)
	if err != nil {
		logrus.WithError(err).Errorf("Cannot parse TEST_DB_LOG_LEVEL %s", dbLogLevel)
		gormLogLevel = logger.Silent
	}

	dsn := os.Getenv("TEST_SIPPY_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_SIPPY_DATABASE_DSN environment variable is not set; skipping database tests")
	}
	dbc, err := db.New(dsn, gormLogLevel)
	if err != nil {
		logrus.WithError(err).Fatal("Cannot connect to database")
	}
	return dbc
}

func GetGcsBucket(t *testing.T) *storage.BucketHandle {
	pathToGcsCredentials := os.Getenv("TEST_GCS_CREDS_PATH")
	if pathToGcsCredentials == "" {
		t.Skip("TEST_GCS_CREDS_PATH environment variable is not set; skipping GCS tests")
	}
	gcsClient, err := gcs.NewGCSClient(context.TODO(), pathToGcsCredentials, "")
	if err != nil {
		logrus.WithError(err).Fatalf("CRITICAL error getting GCS client with credentials at %s", pathToGcsCredentials)
	}
	return gcsClient.Bucket(GcsBucketRoot)
}
