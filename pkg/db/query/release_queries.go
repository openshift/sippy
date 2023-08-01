package query

import (
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

type Release struct {
	Release string
}

func ReleasesFromDB(dbClient *db.DB) ([]Release, error) {
	var releases []Release
	// The string_to_array trick ensures releases are sorted in version order, descending where suffixed releases (-okd)
	// are at the bottom.
	res := dbClient.DB.Raw(`SELECT DISTINCT(release),
    CASE 
        WHEN position('.' in release) != 0 
        THEN string_to_array(split_part(release, '-', 1), '.')::int[] 
    END AS sortable_release, 
    CASE
        WHEN position('-' in release) != 0
        THEN split_part(release, '-', 2)
        ELSE ''
    END AS release_suffix from prow_jobs ORDER BY release_suffix, sortable_release DESC NULLS LAST`).Scan(&releases)
	if res.Error != nil {
		log.Errorf("error querying releases from db: %v", res.Error)
		return releases, res.Error
	}
	return releases, nil
}
