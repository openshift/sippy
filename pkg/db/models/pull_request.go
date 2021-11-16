package models

// PullRequest represents a pull request that was included for the first time
// in a release payload.
type PullRequest struct {
	ID int `bigquery:"id" json:"id" gorm:"primaryKey,column:id"`

	// PullRequestID contains the ID of the GitHub pull request.
	PullRequestID string `bigquery:"pullRequestID" json:"pullRequestID" gorm:"column:pullRequestID"`

	// ReleaseTag is the OpenShift version, e.g. 4.8.0-0.nightly-2021-10-28-013428.
	ReleaseTag string `bigquery:"releaseTag" json:"releaseTag" gorm:"column:releaseTag"`

	// Name contains the names as the repository is known in the release payload.
	Name string `bigquery:"name" json:"name" gorm:"column:name"`

	// Description is the PR description.
	Description string `bigquery:"description" json:"description" gorm:"column:description"`

	// URL is a link to the pull request.
	URL string `bigquery:"url" json:"url" gorm:"column:url"`

	// BugURL links to the bug, if any.
	BugURL string `bigquery:"bugURL" json:"bugURL" gorm:"column:bugURL"`
}
