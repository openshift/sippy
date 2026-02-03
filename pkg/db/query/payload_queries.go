package query

import (
	"time"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/models"
)

func GetPayloadDiff(db *gorm.DB, fromPayload, toPayload string) ([]models.ReleasePullRequest, error) {
	results := make([]models.ReleasePullRequest, 0)
	result := db.Raw(`SELECT url,pull_request_id,name,description,bug_url FROM release_pull_requests 
		WHERE id IN ( SELECT release_pull_request_id FROM release_tag_pull_requests WHERE release_tag_id IN (SELECT id FROM release_tags WHERE release_tag =?)) 
		AND id NOT IN ( SELECT release_pull_request_id FROM release_tag_pull_requests WHERE release_tag_id IN (SELECT id FROM release_tags WHERE release_tag =?)) ORDER BY url`, toPayload, fromPayload).Scan(&results)

	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

// GetLastAcceptedByArchitectureAndStream returns the last accepted payload for each architecture/stream combo.
func GetLastAcceptedByArchitectureAndStream(db *gorm.DB, release string, reportEnd time.Time) ([]models.ReleaseTag, error) {
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
						AND
							release_time < ?
						ORDER BY
							architecture, stream, release_time desc`, release, reportEnd).Scan(&results)

	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

func GetTestFailuresForPayload(db *gorm.DB, payloadTag string) ([]models.PayloadFailedTest, error) {
	results := make([]models.PayloadFailedTest, 0)
	result := db.Raw(`SELECT DISTINCT
	rt.release,
		rt.architecture,
		rt.stream,
		rt.release_tag,
		pjrt.id,
		pjrt.test_id,
		pjrt.suite_id,
		pjrt.status,
		t.name,
		pjrt.prow_job_run_id as prow_job_run_id,
		pjr.url as prow_job_run_url,
		pj.name as prow_job_name
	FROM
	release_tags rt,
		release_job_runs rjr,
		prow_job_run_tests pjrt,
		tests t,
		prow_jobs pj,
		prow_job_runs pjr
	WHERE
	rt.release_tag = ?
	AND rjr.release_tag_id = rt.id
	/*AND rjr.kind = 'Blocking'*/
	AND rjr.State = 'Failed'
	AND pjrt.prow_job_run_id = rjr.prow_job_run_id
	AND pjrt.status = 12
	AND t.id = pjrt.test_id
	AND pjr.id = pjrt.prow_job_run_id
	AND pj.id = pjr.prow_job_id
	ORDER BY pjrt.id DESC`, payloadTag).Scan(&results)

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
func GetLastPayloadTags(db *gorm.DB, release, stream, arch string, reportEnd time.Time) ([]models.ReleaseTag, error) {
	results := []models.ReleaseTag{}

	result := db.Where("release = ?", release).
		Where("stream = ?", stream).
		Where("architecture = ?", arch).
		Where("release_time >= ?", reportEnd.Add(-14*24*time.Hour)).
		Order("release_time DESC").Find(&results)
	if result.Error != nil {
		return nil, result.Error
	}

	return results, nil
}

// GetLastPayloadStatus returns the most recent payload status for an architecture/stream combination,
// as well as the count of how many of the last payloads had that status (e.g., when this returns
// Rejected, 5 -- it means the last 5 payloads were rejected.
func GetLastPayloadStatus(db *gorm.DB, architecture, stream, release string, reportEnd time.Time) (string, int, error) {
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
					architecture = ? AND stream = ? AND release = ? AND release_time <= ?
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
			phase`, architecture, stream, release, reportEnd).Scan(&count)

	return count.Phase, count.Count, result.Error
}

// GetPayloadStreamPhaseCounts returns the number of payloads in each phase for a given stream.
func GetPayloadStreamPhaseCounts(db *gorm.DB, release, architecture, stream string, since *time.Time, reportEnd time.Time) ([]models.PayloadPhaseCount, error) {
	phaseCounts := []models.PayloadPhaseCount{}
	q := db.Table("release_tags").Select("phase, COUNT(phase)").
		Where("release = ? ", release).
		Where("architecture = ?", architecture).
		Where("stream = ?", stream).
		Where("release_time < ?", reportEnd).Group("phase")
	if since != nil {
		q = q.Where("release_time >= ?", *since)
	}
	r := q.Find(&phaseCounts)

	return phaseCounts, r.Error
}

func GetPayloadAcceptanceStatistics(db *gorm.DB, release, architecture, stream string, since *time.Time, reportEnd time.Time) (models.PayloadStatistics, error) {
	results := models.PayloadStatistics{}

	q := db.Table("release_tags").
		Select(`release_time 										                 AS start,
                       LEAD(release_time, 1) OVER (ORDER BY release_time ASC)                AS next_time,
					   LEAD(release_time, 1) OVER (ORDER BY release_time ASC) - release_time AS duration`).
		Where("release = ?", release).
		Where("stream = ?", stream).
		Where("architecture = ?", architecture).
		Where("phase = ?", "Accepted").
		Where("release_time < ?", reportEnd)

	if since != nil {
		q = q.Where("release_time >= ?", *since)
	}

	q = db.Table("(?) as durations", q).
		Select(`EXTRACT(epoch FROM AVG(duration))::bigint as mean_seconds_between,
					   EXTRACT(epoch FROM MIN(duration))::bigint as min_seconds_between,
                       EXTRACT(epoch FROM MAX(duration))::bigint as max_seconds_between`).Scan(&results)

	return results, q.Error
}

// GetReleaseTag returns a release tag by its release_tag string.
func GetReleaseTag(db *gorm.DB, releaseTag string) (*models.ReleaseTag, error) {
	var result models.ReleaseTag
	if err := db.Where("release_tag = ?", releaseTag).First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPreviousPayload returns the payload that immediately precedes the given payload
// in the same release, stream, and architecture by sorting on release_tag.
func GetPreviousPayload(db *gorm.DB, toPayload string) (*models.ReleaseTag, error) {
	// First, look up the toPayload to get its release, stream, and architecture
	var target models.ReleaseTag
	if err := db.Where("release_tag = ?", toPayload).First(&target).Error; err != nil {
		return nil, err
	}

	// Find the previous payload in the same stream by sorting on release_tag
	var result models.ReleaseTag
	if err := db.Where("release = ?", target.Release).
		Where("stream = ?", target.Stream).
		Where("architecture = ?", target.Architecture).
		Where("release_tag < ?", toPayload).
		Order("release_tag DESC").
		First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}
