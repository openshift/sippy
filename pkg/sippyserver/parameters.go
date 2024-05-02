package sippyserver

import (
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/filter"

	"github.com/openshift/sippy/pkg/util"
)

const (
	defaultSortField = "name"
	defaultSort      = apitype.SortDescending
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

func getPeriodDates(defaultPeriod string, req *http.Request, reportEnd time.Time) (start, boundary, end time.Time) {
	period := getPeriod(req, defaultPeriod)

	// If start, boundary, and end params are all specified, use those
	startp := getDateParam("start", req)
	boundaryp := getDateParam("boundary", req)
	endp := getDateParam("end", req)
	if startp != nil && boundaryp != nil && endp != nil {
		return *startp, *boundaryp, *endp
	}

	// Otherwise generate from the period name
	return util.PeriodToDates(period, reportEnd)
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

func getLimitParam(req *http.Request) int {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	return limit
}

func getPaginationParams(req *http.Request) (*apitype.Pagination, error) {
	perPage := req.URL.Query().Get("perPage")
	page := req.URL.Query().Get("page")
	if perPage != "" {
		perPageInt, err := strconv.Atoi(perPage)
		if err != nil {
			return nil, err
		}

		pageNo := 0
		if page != "" {
			pageNo, err = strconv.Atoi(page)
			if err != nil {
				return nil, err
			}
		}

		return &apitype.Pagination{
			PerPage: perPageInt,
			Page:    pageNo,
		}, nil
	}

	return nil, nil
}

func getSortParams(req *http.Request) (string, apitype.Sort) {
	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))
	if sortField == "" {
		sortField = defaultSortField
	}
	if sort == "" {
		sort = defaultSort
	}
	return sortField, sort
}

func splitJobAndJobRunFilters(fil *filter.Filter) (*filter.Filter, *filter.Filter, error) {
	// This function is used by APIs that are largely interested in filtering on the jobs,
	// but there is a case for filtering by the timestamp or build cluster on a job run.
	// Break apart the filter we're given for the respective queries:
	jobFilter := &filter.Filter{
		LinkOperator: fil.LinkOperator,
	}
	jobRunsFilter := &filter.Filter{
		LinkOperator: fil.LinkOperator,
	}
	for _, f := range fil.Items {
		switch {

		case f.Field == "timestamp":
			ms, err := strconv.ParseInt(f.Value, 0, 64)
			if err != nil {
				return nil, nil, err
			}

			f.Value = time.Unix(0, ms*int64(time.Millisecond)).Format("2006-01-02T15:04:05-0700")
			jobRunsFilter.Items = append(jobRunsFilter.Items, f)
		case f.Field == "cluster":
			jobRunsFilter.Items = append(jobRunsFilter.Items, f)
		default:
			jobFilter.Items = append(jobFilter.Items, f)
		}
	}
	return jobFilter, jobRunsFilter, nil
}
