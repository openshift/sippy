package sippyserver

import (
	"net/http"
	"time"
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
