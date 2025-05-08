package jobartifacts

type QueryResponse struct {
	Errors  []JobRunError `json:"errors,omitempty"`
	JobRuns []JobRun      `json:"job_runs,omitempty"`
	// a non-final response ran into timeouts; retrying could get more answers
	IsFinal bool `json:"is_final"`
}

type JobRun struct {
	// ID is string because some parsers translate long ints into scientific notation
	ID      string `json:"id"`
	URL     string `json:"url"`
	JobName string `json:"job_name"`
	// NOTE: limited per maxJobFilesToScan, sets Truncated if more files match
	Artifacts             []JobRunArtifact `json:"artifacts"`
	ArtifactListTruncated bool             `json:"artifact_list_truncated"`
	// a non-final response ran into timeouts; retrying could get more answers
	IsFinal bool `json:"is_final"`
}

type JobRunError struct {
	ID       string `json:"job_run_id,omitempty"`
	Error    string `json:"error"`
	TimedOut bool   `json:"timed_out,omitempty"`
}

type JobRunArtifact struct {
	JobRunID            string      `json:"job_run_id"`
	ArtifactPath        string      `json:"artifact_path"`
	ArtifactContentType string      `json:"artifact_content_type"`
	ArtifactURL         string      `json:"artifact_url"`
	MatchedContent      interface{} `json:"matched_content,omitempty"` // will be one of the content types below
	Error               string      `json:"error,omitempty"`
	TimedOut            bool        `json:"timed_out,omitempty"`
}

type ContentLineMatches struct {
	// NOTE: limited per maxFileMatches, sets Truncated if file has more matches
	Matches   []ContentLineMatch `json:"matches"`
	Truncated bool               `json:"truncated,omitempty"`
}
type ContentLineMatch struct {
	Before []string `json:"before,omitempty"`
	Match  string   `json:"match"`
	After  []string `json:"after,omitempty"`
}
