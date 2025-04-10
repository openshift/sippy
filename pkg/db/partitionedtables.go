package db

import (
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func syncPartitionedTables(db *gorm.DB) error {

	for _, pmv := range PostgresPartitionedTables {
		tableDef := pmv.Definition
		for k, v := range pmv.ReplaceStrings {
			tableDef = strings.ReplaceAll(tableDef, k, v)
		}

		// partitioned tables cannot easily be modified, so there is no change detection here,
		// we just create if not exists the tables every time.
		if res := db.Exec(tableDef); res.Error != nil {
			log.WithError(res.Error).Error("error creating partitioned table")
			return res.Error
		}
		log.Info("created partitioned table: " + pmv.Name)
	}

	return nil
}

// PostgresPartitionedTables are special tables we do not let gorm manage due to
// very specific requirements (partitioning). Schema for these cannot be updated without a
// significant migration due to the partitioning.
var PostgresPartitionedTables = []PostgresView{
	{
		Name:       "test_analysis_by_job_by_dates",
		Definition: testAnalysisByJobByDatesTable,
	},
}

// testAnalysisByJobByDatesTable defines a partitioned table gorm cannot manage for us.
// Partitions are created each time we import a new day during 	the prow data loader.
// The table is used by the analysis by variants view which joins in the job variants.
// WARNING: this schema can't be changed due to the partitioning. complex migration will be required.
const testAnalysisByJobByDatesTable = `
CREATE TABLE IF NOT EXISTS test_analysis_by_job_by_dates (  
     date timestamp with time zone,
     test_id bigint,
     release text,
     job_name text,
     test_name text,
     runs bigint,
     passes bigint,
     flakes bigint,
     failures bigint
 ) PARTITION BY RANGE (date);
CREATE UNIQUE INDEX IF NOT EXISTS test_release_date
ON test_analysis_by_job_by_dates (date, test_id, release, job_name);
CREATE INDEX IF NOT EXISTS test_analysis_name_and_job
ON test_analysis_by_job_by_dates (test_name, job_name);
`
