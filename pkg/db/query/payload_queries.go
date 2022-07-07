package query

import (
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/models"
)

// GetLastAcceptedByArchitectureAndStream returns the last accepted payload for each architecture/stream combo.
func GetLastAcceptedByArchitectureAndStream(db *gorm.DB, release string) ([]models.ReleaseTag, error) {
	results := make([]models.ReleaseTag, 0)

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
							architecture, stream, release_time desc`, release).Scan(&results)

	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

// GetLastOSUpgradeByArchitectureAndStream returns the last release tag that contains an OS upgrade.
func GetLastOSUpgradeByArchitectureAndStream(db *gorm.DB, release string) ([]models.ReleaseTag, error) {
	results := make([]models.ReleaseTag, 0)

	result := db.Raw(`SELECT
						DISTINCT ON
							(architecture, stream)
							*
						FROM
							release_tags
						WHERE
							release = ?
						AND
							previous_os_version != ''
						ORDER BY
							architecture, stream, release_time desc`, release).Scan(&results)

	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

// GetLastPayloadTags returns the most recent payload tags, sorted by date in descending order.
func GetLastPayloadTags(db *gorm.DB, release, stream, arch string, limit int) ([]models.ReleaseTag, error) {
	results := []models.ReleaseTag{}

	result := db.Where("release = ?", release).
		Where("stream = ?", stream).
		Where("architecture = ?", arch).
		Limit(limit).Order("release_time DESC").Find(&results)
	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

func ListPayloadBlockingFailedJobRuns(db *gorm.DB, payloadTagIDs []uint) ([]models.ReleaseJobRun, error) {
	jobRuns := []models.ReleaseJobRun{}
	q := db.Preload("ReleaseTag").Where("release_tag_id in ?", payloadTagIDs).
		Where("kind = 'Blocking'").Where("state = 'Failed'")
	res := q.Find(&jobRuns)
	return jobRuns, res.Error
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
					ROW_NUMBER() OVER(ORDER BY release_time desc) AS id,
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
