package query

import (
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db"
	log "github.com/sirupsen/logrus"
)

func ReleasesFromDB(dbClient *db.DB) ([]v1.Release, error) {
	var releases []v1.Release
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
