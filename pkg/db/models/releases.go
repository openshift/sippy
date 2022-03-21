package models

import (
	"time"

	"gorm.io/gorm"
)

type ReleaseTag struct {
	gorm.Model

	ID int `bigquery:"id" json:"id" gorm:"primaryKey,column:id"`

	// ReleaseTag contains the release version, e.g. 4.8.0-0.nightly-2021-10-28-013428.
	ReleaseTag string `bigquery:"releaseTag" json:"releaseTag" gorm:"column:releaseTag"`

	// Release contains the release X.Y version, e.g. 4.8
	Release string `bigquery:"release" json:"release" gorm:"column:release"`

	// Stream contains the payload stream, e.g. nightly or ci.
	Stream string `bigquery:"stream" json:"stream" gorm:"column:stream"`

	// Architecture contains the arch for a release, e.g. amd64
	Architecture string `bigquery:"architecture" json:"architecture" gorm:"column:architecture"`

	// Phase contains the overall status of a payload: e.g. Ready, Accepted,
	// Rejected. We do not store Ready payloads in bigquery, as we only want
	// the release after it's "fully baked."
	Phase string `bigquery:"phase" json:"phase" gorm:"column:phase"`

	// ReleaseTime contains the timestamp of the release (the suffix of the tag, -YYYY-MM-DD-HHMMSS).
	ReleaseTime time.Time `bigquery:"releaseTime" gorm:"column:releaseTime" json:"releaseTime"`

	// PreviousReleaseTag contains the previously accepted build, on which any
	// changelog is based from.
	PreviousReleaseTag string `bigquery:"previousReleaseTag" json:"previousReleaseTag" gorm:"column:previousReleaseTag"`

	// KubernetesVersion contains the kube version for this payload.
	KubernetesVersion string `bigquery:"kubernetesVersion" json:"kubernetesVersion" gorm:"column:kubernetesVersion"`

	// CurrentOSVersion contains the current machine OS version.
	CurrentOSVersion string `bigquery:"currentOSVersion" json:"currentOSVersion" gorm:"currentOSVersion"`

	// PreviousOSVersion, if any, indicates this release included a machine OS
	// upgrade and this field contains the prior version.
	PreviousOSVersion string `bigquery:"previousOSVersion" json:"previousOSVersion" gorm:"previousOSVersion"`

	// CurrentOSURL is a link to the release page for this machine OS version.
	CurrentOSURL string `bigquery:"currentOSURL" json:"currentOSURL" gorm:"currentOSURL"`

	// PreviousOSURL is a link to the release page for the previous machine OS version.
	PreviousOSURL string `bigquery:"previousOSURL" json:"previousOSURL" gorm:"previousOSURL"`

	// OSDiffURL is a link to the release page diffing the two OS versions.
	OSDiffURL string `bigquery:"osDiffURL" json:"osDiffURL" gorm:"osDiffURL"`

	// PullRequest contains a list of all the PR's in a release.
	PullRequests []PullRequest `gorm:"many2many:release_tag_pull_requests;"`

	Repositories []Repository `gorm:"foreignKey:releaseTagID"`

	JobRuns []JobRun `gorm:"foreignKey:releaseTagID"`
}

// PullRequest represents a pull request that was included for the first time
// in a release payload.
type PullRequest struct {
	gorm.Model

	ID int `bigquery:"id" json:"id" gorm:"primaryKey,column:id"`

	// URL is a link to the pull request.
	URL string `bigquery:"url" json:"url" gorm:"index:pr_url_name,unique;column:url"`

	// PullRequestID contains the ID of the GitHub pull request.
	PullRequestID string `bigquery:"pullRequestID" json:"pullRequestID" gorm:"column:pullRequestID"`

	// Name contains the names as the repository is known in the release payload.
	Name string `bigquery:"name" json:"name" gorm:"index:pr_url_name,unique;column:name"`

	// Description is the PR description.
	Description string `bigquery:"description" json:"description" gorm:"column:description"`

	// BugURL links to the bug, if any.
	BugURL string `bigquery:"bugURL" json:"bugURL" gorm:"column:bugURL"`
}

type Repository struct {
	gorm.Model

	ID           int        `json:"id" gorm:"primaryKey,column:id"`
	Name         string     `json:"name" gorm:"column:name"`
	ReleaseTag   ReleaseTag `gorm:"foreignKey:releaseTagID"`
	ReleaseTagID string     `json:"releaseTag" gorm:"column:releaseTagID"`
	Head         string     `json:"repositoryHead" gorm:"column:repositoryHead"`
	DiffURL      string     `json:"url" gorm:"column:diffURL"`
}

type JobRun struct {
	ID             int        `bigquery:"id" json:"id" gorm:"primaryKey,column:id"`
	ReleaseTag     ReleaseTag `json:"releaseTag" gorm:"foreignKey:releaseTagID"`
	ReleaseTagID   string     `gorm:"column:releaseTagID"`
	Name           string     `bigquery:"name" json:"name" gorm:"column:name"`
	JobName        string     `bigquery:"jobName" json:"jobName" gorm:"column:jobName"`
	Kind           string     `bigquery:"kind" json:"kind" gorm:"column:kind"`
	State          string     `bigquery:"state" json:"state" gorm:"column:state"`
	TransitionTime time.Time  `bigquery:"transitionTime" gorm:"column:transitionTime" json:"transitionTime"`
	Retries        int        `bigquery:"retries" gorm:"column:retries" json:"retries"`
	URL            string     `bigquery:"url" json:"url" gorm:"column:url"`
	UpgradesFrom   string     `bigquery:"upgradesFrom" json:"upgradesFrom" gorm:"column:upgradesFrom"`
	UpgradesTo     string     `bigquery:"upgradesTo" json:"upgradesTo" gorm:"column:upgradesTo"`
	Upgrade        bool       `bigquery:"upgrade" json:"upgrade" gorm:"column:upgrade"`
}

// GetLastAcceptedByArchitectureAndStream returns the last accepted payload for each architecture/stream combo.
func GetLastAcceptedByArchitectureAndStream(db *gorm.DB, release string) ([]ReleaseTag, error) {
	results := make([]ReleaseTag, 0)

	result := db.Raw(`SELECT
						DISTINCT ON
							(architecture, stream)
							*
						FROM
							release_tags
						WHERE
							release = ?
						AND
							phase = 'Accepted'
						ORDER BY
							architecture, stream, "releaseTime" desc`, release).Scan(&results)

	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

// GetLastPayloadStatus returns the most recent payload status for an architecture/stream combination,
// as well as the count of how many of the last payloads had that status (e.g., when this returns
// Rejected, 5 -- it means the last 5 payloads were rejected.
func GetLastPayloadStatus(db *gorm.DB, architecture, stream, release string) (string, int, error) {
	count := struct {
		Phase string `gorm:"column:phase"`
		Count int    `gorm:"column:count"`
	}{}

	result := db.Raw(`
		WITH releases AS
			(
				SELECT
					ROW_NUMBER() OVER(ORDER BY "releaseTime" desc) AS id,
					phase
				FROM
					release_tags
				WHERE
					architecture = ? AND stream = ? AND release = ?
			),
		changes AS
			(
				SELECT
					*,
					CASE WHEN LAG(phase) OVER(ORDER BY id) = phase THEN 0 ELSE 1 END AS change
				FROM
					releases
			),
		groups AS
			(
				SELECT
					*,
					SUM(change) OVER(ORDER BY id) AS group FROM changes
			)
		SELECT
			phase, COUNT(phase)
		FROM
			groups
		WHERE
			groups.group = 1
		GROUP BY
			phase`, architecture, stream, release).Scan(&count)

	return count.Phase, count.Count, result.Error
}
