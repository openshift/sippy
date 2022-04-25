package api

import (
	"fmt"
	"net/http"
	"time"

	"k8s.io/klog"
)

func getTimeParams(w http.ResponseWriter, req *http.Request) (start, boundary, end time.Time, err error) {
	startParam := req.URL.Query().Get("start")
	if startParam != "" {
		start, err = time.Parse("2006-01-02", startParam)
		if err != nil {
			err = fmt.Errorf("error decoding start param: %s", err.Error())
			return
		}
	} else if req.URL.Query().Get("period") == periodTwoDay {
		// twoDay report period starts 9 days ago, (comparing last 2 days vs previous 7)
		start = time.Now().Add(-9 * 24 * time.Hour)
	} else {
		// Default start to 14 days ago
		start = time.Now().Add(-14 * 24 * time.Hour)
	}

	boundaryParam := req.URL.Query().Get("boundary")
	if boundaryParam != "" {
		boundary, err = time.Parse("2006-01-02", boundaryParam)
		if err != nil {
			err = fmt.Errorf("error decoding boundary param: %s", err.Error())
			return
		}
		// We want the boundary to include the entire day specified
		boundary = boundary.Add(24 * time.Hour)
	} else if req.URL.Query().Get("period") == periodTwoDay {
		boundary = time.Now().Add(-2 * 24 * time.Hour)
	} else {
		// Default boundary to 7 days ago
		boundary = time.Now().Add(-7 * 24 * time.Hour)
	}

	endParam := req.URL.Query().Get("end")
	if endParam != "" {
		end, err = time.Parse("2006-01-02", endParam)
		if err != nil {
			err = fmt.Errorf("error decoding end param: %s", err.Error())
			return
		}
		// We want the end date to include the entire day specified
		end = end.Add(24 * time.Hour)
	} else {
		end = time.Now()
	}

	klog.V(4).Infof("Querying between %s -> %s -> %s", start.Format(time.RFC3339), boundary.Format(time.RFC3339), end.Format(time.RFC3339))
	return start, boundary, end, nil
}
