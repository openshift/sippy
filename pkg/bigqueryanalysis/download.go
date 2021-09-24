package bigqueryanalysis

import (
	"context"
	"encoding/gob"
	"os"
	"path"

	"k8s.io/klog"

	v1 "github.com/openshift/sippy/pkg/apis/bigquery/v1"
)

var (
	jobStorage = "big-query-jobs.bin"
)

// DownloadData retrieves data from the BigQuery tables and persists to disk.
func DownloadData(dashboards []string, storagePath string) {
	ctx := context.Background()
	client, err := New(ctx)
	if err != nil {
		klog.Warningf("Could not create BigQuery client, skipping: %s", err.Error())
		return
	}

	jobs, err := client.GetJobs(ctx, dashboards)
	if err != nil {
		klog.Warningf("Could not fetch BigQuery jobs, skipping: %s", err.Error())
		return
	}

	file, err := os.Create(path.Join(storagePath, jobStorage))
	if err != nil {
		panic(err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(jobs); err != nil {
		panic(err)
	}
}

// LoadDataFromDisk reads BigQuery table data from disk into structs.
func LoadDataFromDisk(storagePath string) (jobs []v1.Job, err error) {
	file, err := os.Open(path.Join(storagePath, jobStorage))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&jobs); err != nil {
		return nil, err
	}

	return
}
