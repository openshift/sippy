package prow

import "time"

// ProwJobState specifies whether the job is running
type ProwJobState string

// Various job states.
const (
	// TriggeredState means the job has been created but not yet scheduled.
	TriggeredState ProwJobState = "triggered"
	// PendingState means the job is currently running and we are waiting for it to finish.
	PendingState ProwJobState = "pending"
	// SuccessState means the job completed without error (exit 0)
	SuccessState ProwJobState = "success"
	// FailureState means the job completed with errors (exit non-zero)
	FailureState ProwJobState = "failure"
	// AbortedState means prow killed the job early (new commit pushed, perhaps).
	AbortedState ProwJobState = "aborted"
	// ErrorState means the job could not schedule (bad config, perhaps).
	ErrorState ProwJobState = "error"
)

type ProwJobSpec struct {
	Type    string `json:"type,omitempty"`
	Cluster string `json:"cluster,omitempty"`
	Job     string `json:"job,omitempty"`
}

type ProwJobStatus struct {
	StartTime        time.Time               `json:"startTime,omitempty"`
	PendingTime      *time.Time              `json:"pendingTime,omitempty"`
	CompletionTime   *time.Time              `json:"completionTime,omitempty"`
	State            ProwJobState            `json:"state,omitempty"`
	Description      string                  `json:"description,omitempty"`
	URL              string                  `json:"url,omitempty"`
	PodName          string                  `json:"pod_name,omitempty"`
	BuildID          string                  `json:"build_id,omitempty"`
	JenkinsBuildID   string                  `json:"jenkins_build_id,omitempty"`
	PrevReportStates map[string]ProwJobState `json:"prev_report_states,omitempty"`
}

type ProwJob struct {
	Spec   ProwJobSpec   `json:"spec,omitempty"`
	Status ProwJobStatus `json:"status,omitempty"`
}
