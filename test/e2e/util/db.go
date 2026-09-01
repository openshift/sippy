package util

import (
	"os"
	"testing"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func CreateE2EPostgresConnection(t *testing.T) *db.DB {
	if os.Getenv("SIPPY_E2E_DSN") == "" {
		// Our e2e presubmit cannot expose postgresql externally, these tests are n
		// only useful for local development.
		t.Skip("SIPPY_E2E_DSN environment variable not set, skipping test")
	}

	dbc, err := db.New(os.Getenv("SIPPY_E2E_DSN"), logger.Info)
	require.NoError(t, err, "error connecting to db")

	// Simple check that someone doesn't accidentally run the e2es against the prod db:
	var totalRegressions int64
	dbc.DB.Model(&models.TestRegression{}).Count(&totalRegressions)
	require.Less(t, int(totalRegressions), 300, "found too many test regressions in db, possible indicator someone is running e2e against prod, please clean out test_regressions if this is not the case")

	return dbc
}

func SeedSymptom(t *testing.T, dbc *db.DB, id, summary string) *jobrunscan.Symptom {
	sym := &jobrunscan.Symptom{
		SymptomContent: jobrunscan.SymptomContent{
			ID:          id,
			Summary:     summary,
			MatcherType: jobrunscan.MatcherTypeString,
			MatchString: "e2e-test-match",
		},
	}
	res := dbc.DB.Where("id = ?", id).FirstOrCreate(sym)
	require.NoError(t, res.Error)
	return sym
}

func CleanupSymptoms(dbc *db.DB, ids ...string) {
	for _, id := range ids {
		dbc.DB.Where("id = ?", id).Delete(&jobrunscan.Symptom{})
	}
}

func CleanupTriageSymptoms(dbc *db.DB) {
	res := dbc.DB.Where("1 = 1").Delete(&models.TriageSymptom{})
	if res.Error != nil {
		log.Errorf("error deleting triage symptoms: %v", res.Error)
	}
}
