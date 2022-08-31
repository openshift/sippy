package query

import (
	"time"

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

// GetLastPayloadTags returns payloads tags for last two weeks, sorted by date in descending order.
func GetLastPayloadTags(db *gorm.DB, release, stream, arch string, timeNow time.Time) ([]models.ReleaseTag, error) {
	results := []models.ReleaseTag{}

	result := db.Where("release = ?", release).
		Where("stream = ?", stream).
		Where("architecture = ?", arch).
		Where("release_time >= ?", timeNow.Add(-14*24*time.Hour)).
		Order("release_time DESC").Find(&results)
	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

// GetLastPayloadStatus returns the most recent payload status for an architecture/stream combination,
// as well as the count of how many of the last payloads had that status (e.g., when this returns
// Rejected, 5 -- it means the last 5 payloads were rejected.
func GetLastPayloadStatus(db *gorm.DB, architecture, stream, release string) (string, int, error) {
	count := models.PayloadPhaseCount{}

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

// GetPayloadStreamPhaseCounts returns the number of payloads in each phase for a given stream.
func GetPayloadStreamPhaseCounts(db *gorm.DB, release, architecture, stream string, since *time.Time) ([]models.PayloadPhaseCount, error) {
	phaseCounts := []models.PayloadPhaseCount{}
	q := db.Table("release_tags").Select("phase, COUNT(phase)").
		Where("release = ? ", release).
		Where("architecture = ?", architecture).
		Where("stream = ?", stream).Group("phase")
	if since != nil {
		q = q.Where("release_time >= ?", *since)
	}
	r := q.Find(&phaseCounts)

	return phaseCounts, r.Error
}

func GetPayloadAcceptanceStatistics(db *gorm.DB, release, architecture, stream string, since *time.Time) (models.PayloadStatistics, error) {
	results := models.PayloadStatistics{}

	q := db.Table("release_tags").
		Select(`release_time 										                 AS start,
                       LEAD(release_time, 1) OVER (ORDER BY release_time ASC)                AS next_time,
					   LEAD(release_time, 1) OVER (ORDER BY release_time ASC) - release_time AS duration`).
		Where("release = ?", release).
		Where("stream = ?", stream).
		Where("architecture = ?", architecture).
		Where("phase = ?", "Accepted")

	if since != nil {
		q = q.Where("release_time >= ?", *since)
	}

	q = db.Table("(?) as durations", q).
		Select(`EXTRACT(epoch FROM AVG(duration))::bigint as mean_seconds_between,
					   EXTRACT(epoch FROM MIN(duration))::bigint as min_seconds_between,
                       EXTRACT(epoch FROM MAX(duration))::bigint as max_seconds_between`).Scan(&results)

	return results, q.Error
}
