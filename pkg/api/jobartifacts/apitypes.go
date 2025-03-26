package jobartifacts

type QueryResponse struct {
	Errors  []JobRun `json:"errors"`
	JobRuns []JobRun `json:"jobRuns"`
}

type JobRun struct {
	ID                    int64            `json:"id"`
	URL                   string           `json:"url"`
	JobName               string           `json:"jobName"`
	ArtifactListTruncated bool             `json:"artifactListTruncated"`
	Artifacts             []JobRunArtifact `json:"artifacts"`
	Error                 string           `json:"error,omitempty"`
}

type JobRunArtifact struct {
	JobRunID         int64    `json:"jobRunId"`
	ArtifactURL      string   `json:"artifactUrl"`
	MatchesTruncated bool     `json:"matchesTruncated"`
	MatchedContent   []string `json:"matchedContents"`
}
