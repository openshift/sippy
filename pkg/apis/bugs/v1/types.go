package v1

import "time"

// Bug is used to represent bugs in some serialized content.  It also tracks some additional metadata.
type Bug struct {
	BugzillaBug  `json:",inline"`
	Url          string `json:"url"`
	FailureCount int    `json:"failureCount,omitempty"`
	FlakeCount   int    `json:"flakeCount,omitempty"`
}

// BugzillaBug matches the bugzilla API.  We cannot change this and should consider writing a converter instead of having
// a serialization we don't own.
type BugzillaBug struct {
	ID             int64     `json:"id"`
	Status         string    `json:"status"`
	LastChangeTime time.Time `json:"last_change_time"`
	Summary        string    `json:"summary"`
	TargetRelease  []string  `json:"target_release"`
	Component      []string  `json:"component"`
}
