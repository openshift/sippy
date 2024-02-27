package query

import (
	"github.com/openshift/sippy/pkg/db"
	log "github.com/sirupsen/logrus"
)

type Release struct {
	Release string
	// Status is the release status defined in the BigQuery Releases table. SQL does not have
	// this field
	Status string
}

func ReleasesFromDB(dbClient *db.DB) ([]Release, error) {
	var releases []Release
	// The string_to_array trick ensures releases are sorted in version order, descending
	res := dbClient.DB.Raw(`
		SELECT DISTINCT(release), case when position('.' in release) != 0 then string_to_array(release, '.')::int[] end as sortable_release
                FROM prow_jobs
                ORDER BY sortable_release desc NULLS LAST`).Scan(&releases)
	if res.Error != nil {
		log.Errorf("error querying releases from db: %v", res.Error)
		return releases, res.Error
	}
	return releases, nil
}
