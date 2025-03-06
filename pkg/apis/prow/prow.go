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

type Refs struct {
	// Org is something like kubernetes or k8s.io
	Org string `json:"org"`
	// Repo is something like test-infra
	Repo string `json:"repo"`
	// RepoLink links to the source for Repo.
	RepoLink string `json:"repo_link,omitempty"`

	BaseRef string `json:"base_ref,omitempty"`
	BaseSHA string `json:"base_sha,omitempty"`
	// BaseLink is a link to the commit identified by BaseSHA.
	BaseLink string `json:"base_link,omitempty"`

	Pulls []Pull `json:"pulls,omitempty"`
}

// Pull describes a pull request at a particular point in time.
type Pull struct {
	Number int    `json:"number"`
	Author string `json:"author"`
	SHA    string `json:"sha"`
	Title  string `json:"title,omitempty"`

	// Ref is git ref can be checked out for a change
	// for example,
	// github: pull/123/head
	// gerrit: refs/changes/00/123/1
	Ref string `json:"ref,omitempty"`
	// Link links to the pull request itself.
	Link string `json:"link,omitempty"`
	// CommitLink links to the commit identified by the SHA.
	CommitLink string `json:"commit_link,omitempty"`
	// AuthorLink links to the author of the pull request.
	AuthorLink string `json:"author_link,omitempty"`
}

type ProwJobSpec struct {
	Type             string           `json:"type,omitempty"`
	Cluster          string           `json:"cluster,omitempty"`
	Job              string           `json:"job,omitempty"`
	DecorationConfig DecorationConfig `json:"decoration_config,omitempty"`

	// Refs is the code under test, determined at runtime by Prow itself
	Refs *Refs `json:"refs,omitempty"`
}

type GCSConfiguration struct {
	Bucket string `json:"bucket"`
}

type DecorationConfig struct {
	GCSConfiguration GCSConfiguration `json:"gcs_configuration"`
}

type Spec struct {
	DecorationConfig DecorationConfig `json:"decoration_config"`
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
	PrevReportStates map[string]ProwJobState `json:"prev_report_states,omitempty"`
}

type ProwJob struct {
	Spec   ProwJobSpec   `json:"spec,omitempty"`
	Status ProwJobStatus `json:"status,omitempty"`
}
