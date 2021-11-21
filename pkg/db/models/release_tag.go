package models

import (
	"time"

	"gorm.io/gorm"
)

type ReleaseTag struct {
	ID int `bigquery:"id" json:"id" gorm:"primaryKey,column:id"`

	// Phase contains the overall status of a payload: e.g. Ready, Accepted,
	// Rejected. We do not store Ready payloads in bigquery, as we only want
	// the release after it's "fully baked."
	Phase string `bigquery:"phase" json:"phase" gorm:"column:phase"`

	// Release contains the release X.Y version, e.g. 4.8
	Release string `bigquery:"release" json:"release" gorm:"column:release"`

	// Stream contains the payload stream, e.g. nightly or ci.
	Stream string `bigquery:"stream" json:"stream" gorm:"column:stream"`

	// ReleaseTag contains the release version, e.g. 4.8.0-0.nightly-2021-10-28-013428.
	ReleaseTag string `bigquery:"releaseTag" json:"releaseTag" gorm:"column:releaseTag"`

	// Architecture contains the arch for a release, e.g. amd64
	Architecture string `bigquery:"architecture" json:"architecture" gorm:"column:architecture"`

	// ReleaseTime contains the timestamp of the release (the suffix of the tag, -YYYY-MM-DD-HHMMSS).
	ReleaseTime time.Time `bigquery:"releaseTime" gorm:"column:releaseTime" json:"releaseTime"`

	// PreviousReleaseTag contains the previously accepted build, on which any
	// changelog is based from.
	PreviousReleaseTag string `bigquery:"previousReleaseTag" json:"previousReleaseTag" gorm:"column:previousReleaseTag"`

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
