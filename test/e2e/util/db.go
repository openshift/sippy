package util

import (
	"os"
	"testing"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func CreateE2EPostgresConnection(t *testing.T) *db.DB {
	require.NotEqual(t, "", os.Getenv("SIPPY_E2E_DSN"),
		"SIPPY_E2E_DSN environment variable not set")

	dbc, err := db.New(os.Getenv("SIPPY_E2E_DSN"), logger.Info)
	require.NoError(t, err, "error connecting to db")

	// Simple check that someone doesn't accidentally run the e2es against the prod db:
	var totalRegressions int64
	dbc.DB.Model(&models.TestRegression{}).Count(&totalRegressions)
	require.Less(t, int(totalRegressions), 300, "found too many test regressions in db, possible indicator someone is running e2e against prod, please clean out test_regressions if this is not the case")

	return dbc
}
