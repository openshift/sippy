package v1

import "time"

type Job struct {
	ID                          int64
	JobName                     string
	GCSBucketName               string
	GCSJobHistoryLocationPrefix string
	CollectDisruption           bool
	CollectTestRuns             bool
	Platform                    string
	Network                     string
	IPMode                      string
	Topology                    string
	Release                     string
	FromRelease                 string
	RunsUpgrade                 bool
	RunsE2EParallel             bool
	RunsE2ESerial               bool
}

type JobRun struct {
	Name       string
	JobName    string
	Status     string
	StartTime  time.Time
	EndTime    time.Time
	ReleaseTag string
	Cluster    string
}
