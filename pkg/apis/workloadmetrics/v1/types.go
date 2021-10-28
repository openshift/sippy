// workloadmetrics package contains types used for processing CPU and memory usage
// results from the OpenShift perfscale team elasticsearch.
package v1

import (
	"fmt"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

type WorkloadMetricsRow struct {
	ID                int    `json:"id"`
	Workload          string `json:"workload"`
	UpstreamJob       string `json:"upstreamJob"`
	Release           string `json:"release"`
	NetworkType       string `json:"networkType"`
	Platform          string `json:"platform"`
	ControlPlaneCount int64  `json:"controlPlaneCount"`
	InfraCount        int64  `json:"infraCount"`
	WorkerCount       int64  `json:"workerCount"`

	CurrentStart             time.Time `json:"currentStart"`
	PreviousStart            time.Time `json:"previousStart"`
	CurrentEnd               time.Time `json:"currentEnd"`
	PreviousEnd              time.Time `json:"previousEnd"`
	CurrentPassCount         int64     `json:"currentPassCount"`
	PreviousPassCount        int64     `json:"previousPassCount"`
	CurrentFailCount         int64     `json:"currentFailCount"`
	PreviousFailCount        int64     `json:"previousFailCount"`
	CurrentPassTotalDur      int64     `json:"-"`
	PreviousPassTotalDur     int64     `json:"-"`
	CurrentPassAvgDur        int64     `json:"currentPassAvgDur"`
	PreviousPassAvgDur       int64     `json:"previousPassAvgDur"`
	CurrentTotalCPUCount     int64     `json:"currentTotalCPUCount"`
	PreviousTotalCPUCount    int64     `json:"previousTotalCPUCount"`
	CurrentAvgTotalCPU       float64   `json:"-"`
	PreviousAvgTotalCPU      float64   `json:"-"`
	CurrentMaxTotalCPU       float64   `json:"-"`
	PreviousMaxTotalCPU      float64   `json:"-"`
	CurrentAvgCPU            float64   `json:"currentAvgCPU"`
	PreviousAvgCPU           float64   `json:"previousAvgCPU"`
	CurrentMaxCPU            float64   `json:"currentMaxCPU"`
	PreviousMaxCPU           float64   `json:"previousMaxCPU"`
	CurrentTotalMemCount     int64     `json:"-"`
	PreviousTotalMemCount    int64     `json:"-"`
	CurrentAvgTotalMemBytes  float64   `json:"-"`
	PreviousAvgTotalMemBytes float64   `json:"-"`
	CurrentMaxTotalMemBytes  float64   `json:"-"`
	PreviousMaxTotalMemBytes float64   `json:"-"`
	CurrentAvgMemBytes       float64   `json:"currentAvgMem"`
	PreviousAvgMemBytes      float64   `json:"previousAvgMem"`
	CurrentMaxMemBytes       float64   `json:"currentMaxMem"`
	PreviousMaxMemBytes      float64   `json:"previousMaxMem"`
}

// Process result will examine a match from elasticsearch and sort it into the appropriate bucket
// if it's timestamp falls in the correct range, otherwise discard it if no matching bucket can be
// found. (though this may indicate a programmer error as our ES query should limit by the correct
// bucket ranges)
func (wm *WorkloadMetricsRow) ProcessResult(hitMap, workloadCPU, workloadMem map[string]gjson.Result) error {
	hitMetadata := hitMap["metadata"].Map()
	tsStr := hitMetadata["start_date"].String()
	timestamp, err := time.Parse(
		time.RFC3339Nano,
		tsStr)
	if err != nil {
		return errors.Wrap(err, "unable to parse job run timestamp")
	}

	var passCount, failCount, passTotalDurationSeconds *int64
	var cpuCount, memCount *int64
	var cpuAvgTotal, cpuMaxTotal, memAvgTotal, memMaxTotal *float64

	if timestamp.Before(wm.PreviousStart) {
		return fmt.Errorf("result job %s result %s timestamp %s is before prevPeriodStart %s, should not be present in query, possibly bug",
			hitMetadata["upstream_job"].String(), hitMetadata["uuid"].String(), timestamp.Format(time.RFC3339), wm.PreviousStart.Format(time.RFC3339))
	}

	if timestamp.After(wm.CurrentStart) {
		passCount = &wm.CurrentPassCount
		failCount = &wm.CurrentFailCount
		passTotalDurationSeconds = &wm.CurrentPassTotalDur
		cpuCount = &wm.CurrentTotalCPUCount
		memCount = &wm.CurrentTotalMemCount
		cpuAvgTotal = &wm.CurrentAvgTotalCPU
		cpuMaxTotal = &wm.CurrentMaxTotalCPU
		memAvgTotal = &wm.CurrentAvgTotalMemBytes
		memMaxTotal = &wm.CurrentMaxTotalMemBytes
	} else {
		passCount = &wm.PreviousPassCount
		failCount = &wm.PreviousFailCount
		passTotalDurationSeconds = &wm.PreviousPassTotalDur
		cpuCount = &wm.PreviousTotalCPUCount
		memCount = &wm.PreviousTotalMemCount
		cpuAvgTotal = &wm.PreviousAvgTotalCPU
		cpuMaxTotal = &wm.PreviousMaxTotalCPU
		memAvgTotal = &wm.PreviousAvgTotalMemBytes
		memMaxTotal = &wm.PreviousMaxTotalMemBytes
	}

	durSeconds := hitMetadata["job_duration"].Int()
	status := hitMetadata["job_status"].String()
	switch status {
	case "success":
		*passCount++
		*passTotalDurationSeconds += durSeconds // only tracked for successes
	case "upstream_failed":
		*failCount++
	case "failed":
		*failCount++
	case "default":
		return fmt.Errorf("perfscale job %s has unknown job_success field: %s", hitMetadata["uuid"].String(), status)
	}

	*cpuCount++
	*cpuAvgTotal += workloadCPU["average"].Float()
	*cpuMaxTotal += workloadCPU["max"].Float()

	*memCount++
	*memAvgTotal = +workloadMem["average"].Float()
	*memMaxTotal = +workloadMem["max"].Float()

	return nil
}

func (wm *WorkloadMetricsRow) GetFieldType(param string) api.ColumnType {
	switch param {
	case "workload", "upstreamJob", "release", "networkType", "platform":
		return api.ColumnTypeString
	default:
		return api.ColumnTypeNumerical
	}
}

func (wm *WorkloadMetricsRow) GetStringValue(param string) (string, error) {
	switch param {
	case "workload":
		return wm.Workload, nil
	case "upstreamJob":
		return wm.UpstreamJob, nil
	case "release":
		return wm.Release, nil
	case "networkType":
		return wm.NetworkType, nil
	case "platform":
		return wm.Platform, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (wm *WorkloadMetricsRow) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "id":
		return float64(wm.ID), nil
	case "currentAvgCPU":
		return wm.CurrentAvgCPU, nil
	case "previousAvgCPU":
		return wm.PreviousAvgCPU, nil
	case "currentAvgMem":
		return wm.CurrentAvgMemBytes, nil
	case "previousAvgMem":
		return wm.PreviousAvgMemBytes, nil
	case "currentMaxCPU":
		return wm.CurrentMaxCPU, nil
	case "previousMaxCPU":
		return wm.PreviousMaxCPU, nil
	case "currentMaxMem":
		return wm.CurrentMaxMemBytes, nil
	case "previousMaxMem":
		return wm.PreviousAvgMemBytes, nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (wm *WorkloadMetricsRow) GetArrayValue(param string) ([]string, error) {
	return nil, fmt.Errorf("unknown array value field %s", param)
}
