package query

import (
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	"k8s.io/klog"
)

func JobReports(dbc *db.DB, filterOpts *filter.FilterOptions, release string, start, boundary, end time.Time) ([]apitype.Job, error) {
	now := time.Now()
	jobReports := make([]apitype.Job, 0)

	table := dbc.DB.Table("job_results(?, ?, ?, ?)", release, start, boundary, end)
	if table.Error != nil {
		return jobReports, table.Error
	}

	q, err := filter.FilterableDBResult(table, filterOpts, apitype.Job{})
	if err != nil {
		return jobReports, err
	}

	q.Scan(&jobReports)
	elapsed := time.Since(now)
	klog.Infof("BuildJobResult completed in %s with %d results from db", elapsed, len(jobReports))

	// FIXME(stbenjam): There's a UI bug where the jobs page won't load if either bugs filled is "null"
	// instead of empty array. Quick hack to make this work.
	for i, j := range jobReports {
		if len(j.Bugs) == 0 {
			jobReports[i].Bugs = make([]bugsv1.Bug, 0)
		}

		if len(j.AssociatedBugs) == 0 {
			jobReports[i].AssociatedBugs = make([]bugsv1.Bug, 0)
		}
	}

	return jobReports, nil
}
