package bigqueryanalysis

import (
	bigqueryv1 "github.com/openshift/sippy/pkg/apis/bigquery/v1"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/util"
)

// InsertBigQueryDataToJobs adds data from the bigquery tables to the JobResult data from TestGrid.
func InsertBigQueryDataToJobs(bigqueryJobs []bigqueryv1.Job, sippyJobs []sippyv1.JobResult) []sippyv1.JobResult {
	for _, bqj := range bigqueryJobs {
		job := util.FindJobResultForJobName(bqj.JobName, sippyJobs)
		if job == nil {
			continue
		}

		job.GCSBucketName = bqj.GCSBucketName
		job.GCSJobHistoryLocationPrefix = bqj.GCSJobHistoryLocationPrefix
		job.IPMode = bqj.IPMode
		job.Network = bqj.Network
		job.RunsE2EParallel = bqj.RunsE2EParallel
		job.RunsE2ESerial = bqj.RunsE2ESerial
		job.RunsUpgrade = bqj.RunsUpgrade
		job.Topology = bqj.Topology
	}

	return sippyJobs
}
