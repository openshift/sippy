package stagingtablemanager

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

// StagingTableManager can used to create an empty staging table for table-based upsert operations.
type StagingTableManager struct {
	dbc          *db.DB
	tableName    string
	stagingTable string
}

func New(dbc *db.DB, tableName string) *StagingTableManager {
	return &StagingTableManager{
		dbc:          dbc,
		tableName:    tableName,
		stagingTable: fmt.Sprintf("%s_staging_%d", tableName, time.Now().UnixMilli()),
	}
}

// CreateStagingTable creates the temporary staging table.  Use dbc.Table(stagingTable).Save(X) to write records
// to the temporary staging table instead of the default.
func (stm *StagingTableManager) CreateStagingTable() (string, error) {
	// Create a temporary staging table
	if err := stm.dbc.DB.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s AS TABLE %s WITH NO DATA", stm.stagingTable, stm.tableName)).Error; err != nil {
		return "", err
	}

	return stm.stagingTable, nil
}

// Cleanup removes any leftover tables.  Good idea to defer this after calling CreateStagingTable.
func (stm *StagingTableManager) Cleanup() error {
	log.Infof("dropping staging table %q", stm.stagingTable)
	err := stm.dbc.DB.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", stm.stagingTable)).Error
	if err != nil {
		log.WithError(err).Warningf("couldn't drop staging table %q", stm.stagingTable)
	}
	return err
}
