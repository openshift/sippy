package sippyserver

import (
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

func getISO8601Date(paramName string, req *http.Request) (*time.Time, error) {
	param := req.URL.Query().Get(paramName)
	if param == "" {
		return nil, nil
	}

	date, err := time.Parse("2006-01-02T15:04:05Z", param)
	if err != nil {
		return nil, err
	}

	return &date, nil
}

func getPeriodDates(defaultPeriod string, req *http.Request) (start, boundary, end time.Time) {
	period := getPeriod(req, defaultPeriod)

	startp := getDateParam("start", req)
	boundaryp := getDateParam("boundary", req)
	endp := getDateParam("end", req)
	if startp != nil && boundaryp != nil && endp != nil {
		return *startp, *boundaryp, *endp
	}

	if period == "twoDay" {
		start = time.Now().Add(-9 * 24 * time.Hour)
		boundary = time.Now().Add(-2 * 24 * time.Hour)

	} else {
		start = time.Now().Add(-14 * 24 * time.Hour)
		boundary = time.Now().Add(-7 * 24 * time.Hour)
	}
	end = time.Now()

	return start, boundary, end
}

func getDateParam(paramName string, req *http.Request) *time.Time {
	param := req.URL.Query().Get(paramName)
	if param != "" {
		t, err := time.Parse("2006-01-02", param)
		if err != nil {
			log.WithError(err).Warningf("error decoding %q param: %s", param, err.Error())
			return nil
		}
		return &t
	}

	return nil
}

func getPeriod(req *http.Request, defaultValue string) string {
	period := req.URL.Query().Get("period")
	if period == "" {
		return defaultValue
	}
	return period
}
