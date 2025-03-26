package jobartifacts

type QueryResponse struct {
	Errors  []JobRunError `json:"errors,omitempty"`
	JobRuns []JobRun      `json:"jobRuns,omitempty"`
}

type JobRun struct {
	// ID is string because some parsers translate long ints into scientific notation
	ID      string `json:"id"`
	URL     string `json:"url"`
	JobName string `json:"jobName"`
	// NOTE: limited per maxJobFilesToScan, sets Truncated if more files match
	Artifacts             []JobRunArtifact `json:"artifacts"`
	ArtifactListTruncated bool             `json:"artifactListTruncated"`
}

type JobRunError struct {
	ID    string `json:"jobRunId,omitempty"`
	Error string `json:"error"`
}

type JobRunArtifact struct {
	JobRunID    string `json:"jobRunId"`
	ArtifactURL string `json:"artifactUrl"`
	// NOTE: limited per maxFileMatches, sets Truncated if file has more matches
	MatchedContent   []string `json:"matchedContents,omitempty"`
	MatchesTruncated bool     `json:"matchesTruncated"`
	Error            string   `json:"error,omitempty"`
}
