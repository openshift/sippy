package util

import (
	"context"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
)

/*
	  Enable functional tests requiring a live database and/or GCS bucket with known data to run, but
	  don't risk checking in credentials with code; supply them via environment variables.
		TEST_GCS_CREDS_PATH: the path to a local GCS credentials file, e.g. /home/$USER/git/sippy/openshift-sippy-ro.creds.json
		TEST_SIPPY_DATABASE_DSN: the DSN for the sippy postgres database e.g. postgresql://sippyro:...@sippy-postgresql...amazonaws.com/sippy_openshift
		TEST_DB_LOG_LEVEL: "silent" or "info" or "warn" or "error" - the log level for gorm database methods
	  We do not want these trying to run during CI; skip tests with required environment variables that are not set.
*/

const GcsBucketRoot = "test-platform-results"

func GetDbHandle(t *testing.T) *db.DB {
	dbLogLevel := os.Getenv("TEST_DB_LOG_LEVEL") // e.g. "info" or "silent"
	if dbLogLevel == "" {
		dbLogLevel = "silent"
	}
	gormLogLevel, err := db.ParseGormLogLevel(dbLogLevel)
	if err != nil {
		logrus.WithError(err).Errorf("Cannot parse TEST_DB_LOG_LEVEL %s", dbLogLevel)
		gormLogLevel = logger.Silent
	}

	dsn := os.Getenv("TEST_SIPPY_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_SIPPY_DATABASE_DSN environment variable is not set; skipping database tests")
	}
	dbc, err := db.New(dsn, gormLogLevel)
	if err != nil {
		logrus.WithError(err).Fatal("Cannot connect to database")
	}
	return dbc
}

func GetGcsBucket(t *testing.T) *storage.BucketHandle {
	pathToGcsCredentials := os.Getenv("TEST_GCS_CREDS_PATH")
	if pathToGcsCredentials == "" {
		t.Skip("TEST_GCS_CREDS_PATH environment variable is not set; skipping GCS tests")
	}
	gcsClient, err := gcs.NewGCSClient(context.TODO(), pathToGcsCredentials, "")
	if err != nil {
		logrus.WithError(err).Fatalf("CRITICAL error getting GCS client with credentials at %s", pathToGcsCredentials)
	}
	return gcsClient.Bucket(GcsBucketRoot)
}

type PseudoCache struct {
	Cache map[string][]byte
}

func (c *PseudoCache) Get(_ context.Context, key string, _ time.Duration) ([]byte, error) {
	return c.Cache[key], nil
}

func (c *PseudoCache) Set(_ context.Context, key string, content []byte, _ time.Duration) error {
	c.Cache[key] = content
	return nil
}
