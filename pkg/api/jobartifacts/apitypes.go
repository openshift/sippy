package jobartifacts

type QueryResponse struct {
	Errors  []JobRunError `json:"errors"`
	JobRuns []JobRun      `json:"jobRuns"`
}

type JobRun struct {
	// ID is string because some parsers translate long ints into scientific notation
	ID                    string           `json:"id"`
	URL                   string           `json:"url"`
	JobName               string           `json:"jobName"`
	ArtifactListTruncated bool             `json:"artifactListTruncated"`
	Artifacts             []JobRunArtifact `json:"artifacts"`
}

type JobRunError struct {
	ID    string `json:"jobRunId,omitempty"`
	Error string `json:"error"`
}

type JobRunArtifact struct {
	JobRunID         string   `json:"jobRunId"`
	ArtifactURL      string   `json:"artifactUrl"`
	MatchesTruncated bool     `json:"matchesTruncated"`
	MatchedContent   []string `json:"matchedContents"`
}
