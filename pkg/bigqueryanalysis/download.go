package bigqueryanalysis

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path"

	v1 "github.com/openshift/sippy/pkg/apis/bigquery/v1"

	"k8s.io/klog"
)

var (
	jobStorage = "big-query-jobs.bin"
	runStorage = "big-query-job-runs.bin"
)

func readJobs(storagePath string) ([]v1.Job, error) {
	jobs := make([]v1.Job, 0)
	file, err := os.Open(path.Join(storagePath, jobStorage))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&jobs); err != nil {
		return nil, err
	}

	return jobs, nil
}

func writeJobs(ctx context.Context, client *Client, storagePath string, dashboards []string) error {
	jobs, err := client.GetJobs(ctx, dashboards)
	if err != nil {
		return fmt.Errorf("could not fetch BigQuery jobs, skipping: %s", err.Error())
	}
	file, err := os.Create(path.Join(storagePath, jobStorage))
	if err != nil {
		panic(err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(jobs)
}

func readJobRuns(storagePath string) ([]v1.JobRun, error) {
	runs := make([]v1.JobRun, 0)
	file, err := os.Open(path.Join(storagePath, runStorage))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&runs); err != nil {
		return nil, err
	}

	return runs, nil
}

func writeJobRuns(ctx context.Context, client *Client, storagePath string, dashboards []string) error {
	runs, err := client.GetJobRuns(ctx, dashboards)
	if err != nil {
		return fmt.Errorf("could not fetch BigQuery job runs, skipping: %s", err.Error())
	}

	file, err := os.Create(path.Join(storagePath, runStorage))
	if err != nil {
		panic(err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(runs)
}

// DownloadData retrieves data from the BigQuery tables and persists to disk.
func DownloadData(dashboards []string, storagePath string) {
	ctx := context.Background()
	client, err := New(ctx)
	if err != nil {
		klog.Warningf("Could not create BigQuery client, skipping: %s", err.Error())
		return
	}

	if err := writeJobs(ctx, client, storagePath, dashboards); err != nil {
		klog.Warningf("Could not fetch BigQuery jobs, skipping: %s", err.Error())
	}

	if err := writeJobRuns(ctx, client, storagePath, dashboards); err != nil {
		klog.Warningf("Could not fetch BigQuery job runs, skipping: %s", err.Error())
	}
}

// LoadDataFromDisk reads BigQuery table data from disk into structs.
func LoadDataFromDisk(storagePath string) ([]v1.Job, []v1.JobRun, error) {
	jobs, err := readJobs(storagePath)
	if err != nil {
		return nil, nil, err
	}

	runs, err := readJobRuns(storagePath)
	if err != nil {
		return nil, nil, err
	}

	return jobs, runs, err
}
