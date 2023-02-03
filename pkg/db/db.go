package db

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db/models"
)

type SchemaHashType string

const (
	hashTypeMatView      SchemaHashType = "matview"
	hashTypeMatViewIndex SchemaHashType = "matview_index"
	hashTypeFunction     SchemaHashType = "function"
)

var matViewRefreshMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "sippy_matview_refresh_millis",
	Help:    "Milliseconds to refresh our postgresql materialized views",
	Buckets: []float64{10, 100, 200, 500, 1000, 5000, 10000, 30000, 60000, 300000},
}, []string{"view"})

var allMatViewsRefreshMetric = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "sippy_all_matviews_refresh_millis",
	Help:    "Milliseconds to refresh our postgresql materialized views",
	Buckets: []float64{5000, 10000, 30000, 60000, 300000, 600000, 1200000, 1800000, 2400000, 3000000, 3600000},
})

type DB struct {
	DB *gorm.DB

	// BatchSize is used for how many insertions we should do at once. Postgres supports
	// a maximum of 2^16 records per insert.
	BatchSize int
}

// log2LogrusWriter bridges gorm logging to logrus logging.
// All messages will come through at DEBUG level.
type log2LogrusWriter struct {
	entry *log.Entry
}

func (w log2LogrusWriter) Printf(msg string, args ...interface{}) {
	w.entry.Debugf(msg, args...)
}

func New(dsn string, logLevel gormlogger.LogLevel) (*DB, error) {
	gormLogger := gormlogger.New(
		log2LogrusWriter{entry: log.WithField("source", "gorm")},
		gormlogger.Config{
			SlowThreshold:             2 * time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, err
	}
	return &DB{
		DB:        db,
		BatchSize: 1024,
	}, nil
}

func (d *DB) UpdateSchema(reportEnd *time.Time) error {

	if err := d.DB.AutoMigrate(&models.ReleaseTag{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ReleasePullRequest{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ReleaseRepository{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ReleaseJobRun{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ProwJob{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ProwJobRun{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.Test{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.Suite{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ProwJobRunTest{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ProwJobRunTestOutput{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ProwJobRunTestOutputMetadata{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.APISnapshot{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.Bug{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.ProwPullRequest{}); err != nil {
		return err
	}

	if err := d.DB.AutoMigrate(&models.SchemaHash{}); err != nil {
		return err
	}

	// TODO: in the future, we should add an implied migration. If we see a new suite needs to be created,
	// scan all test names for any starting with that prefix, and if found merge all records into a new or modified test
	// with the prefix stripped. This is not necessary today, but in future as new suites are added, there'll be a good
	// change this happens without thinking to update sippy.
	if err := populateTestSuitesInDB(d.DB); err != nil {
		return err
	}

	if err := syncPostgresMaterializedViews(d.DB, reportEnd); err != nil {
		return err
	}

	if err := syncPostgresFunctions(d.DB); err != nil {
		return err
	}

	log.Info("db schema updated")

	return nil
}

// refreshMaterializedViews updates the postgresql materialized views backing our reports. It is called by the handler
// for the /refresh API endpoint, which is called by the sidecar script which loads the new data from testgrid into the
// main postgresql tables.
//
// refreshMatviewOnlyIfEmpty is used on startup to indicate that we want to do an initial refresh *only* if
// the views appear to be empty.
func (dbc *DB) refreshMaterializedViews(refreshMatviewOnlyIfEmpty bool) {
	var promPusher *push.Pusher
	if pushgateway := os.Getenv("SIPPY_PROMETHEUS_PUSHGATEWAY"); pushgateway != "" {
		promPusher = push.New(pushgateway, "sippy-matviews")
		promPusher.Collector(matViewRefreshMetric)
		promPusher.Collector(allMatViewsRefreshMetric)
	}

	log.Info("refreshing materialized views")
	allStart := time.Now()

	if dbc == nil {
		log.Info("skipping materialized view refresh as server has no db connection provided")
		return
	}
	// create a channel for work "tasks"
	ch := make(chan string)

	wg := sync.WaitGroup{}

	// allow concurrent workers for refreshing matviews in parallel
	for t := 0; t < 3; t++ {
		wg.Add(1)
		go dbc.refreshMatview(refreshMatviewOnlyIfEmpty, ch, &wg)
	}

	for _, pmv := range PostgresMatViews {
		ch <- pmv.Name
	}

	close(ch)
	wg.Wait()

	allElapsed := time.Since(allStart)
	log.WithField("elapsed", allElapsed).Info("refreshed all materialized views")
	allMatViewsRefreshMetric.Observe(float64(allElapsed.Milliseconds()))

	if promPusher != nil {
		log.Info("pushing metrics to prometheus gateway")
		if err := promPusher.Add(); err != nil {
			log.WithError(err).Error("could not push to prometheus pushgateway")
		} else {
			log.Info("successfully pushed metrics to prometheus gateway")
		}
	}
}

func (dbc *DB) refreshMatview(refreshMatviewOnlyIfEmpty bool, ch chan string, wg *sync.WaitGroup) {
	for matView := range ch {
		start := time.Now()
		tmpLog := log.WithField("matview", matView)

		// If requested, we only refresh the materialized view if it has no rows
		if refreshMatviewOnlyIfEmpty {
			var count int
			if res := dbc.DB.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", matView)).Scan(&count); res.Error != nil {
				tmpLog.WithError(res.Error).Warn("proceeding with refresh of matview that appears to be empty")
			} else if count > 0 {
				tmpLog.Info("skipping matview refresh as it appears to be populated")
				continue
			}
		}

		// Try to refresh concurrently, if we get an error that likely means the view has never been
		// populated (could be a developer env, or a schema migration on the view), fall back to the normal
		// refresh which locks reads.
		tmpLog.Info("refreshing materialized view")
		if res := dbc.DB.Exec(
			fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", matView)); res.Error != nil {
			tmpLog.WithError(res.Error).Warn("error refreshing materialized view concurrently, falling back to regular refresh")

			if res := dbc.DB.Exec(
				fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", matView)); res.Error != nil {
				tmpLog.WithError(res.Error).Error("error refreshing materialized view")
			} else {
				elapsed := time.Since(start)
				tmpLog.WithField("elapsed", elapsed).Info("refreshed materialized view")
				matViewRefreshMetric.WithLabelValues(matView).Observe(float64(elapsed.Milliseconds()))
			}

		} else {
			elapsed := time.Since(start)
			tmpLog.WithField("elapsed", elapsed).Info("refreshed materialized view concurrently")
			matViewRefreshMetric.WithLabelValues(matView).Observe(float64(elapsed.Milliseconds()))
		}
	}
	wg.Done()
}

func (dbc *DB) RefreshData(refreshMatviewsOnlyIfEmpty bool) {
	log.Infof("Refreshing data")
	dbc.refreshMaterializedViews(refreshMatviewsOnlyIfEmpty)
	log.Infof("Refresh complete")
}

// syncSchema will update generic db resources if their schema has changed. (functions, materialized views, indexes)
// This is useful for resources that cannot be updated incrementally with goose, and can cause conflict / last write
// wins problems with concurrent development.
//
// desiredSchema should be the full SQL command we would issue to create the resource fresh. It will be hashed and
// compared to a pre-existing value in the db of the given name and type, if any exists. If none exists, or the hashes
// have changed, the resource will be recreated.
//
// dropSQL is the full SQL command we will run if we detect that the resource needs updating. It should include
// "IF EXISTS" as it will be attempted even when no previous resource exists. (i.e. new databases)
//
// This function does not check for existence of the resource in the db, thus if you ever delete something manually, it will
// not be recreated until you also delete the corresponding row from schema_hashes.
//
// returns true if schema change was detected
func syncSchema(db *gorm.DB, hashType SchemaHashType, name, desiredSchema, dropSQL string, forceUpdate bool) (bool, error) {

	// Calculate hash of our schema to see if anything has changed.
	hash := sha256.Sum256([]byte(desiredSchema))
	hashStr := base64.URLEncoding.EncodeToString(hash[:])
	vlog := log.WithFields(log.Fields{"name": name, "type": hashType})
	vlog.WithField("hash", hashStr).Debug("generated SHA256 hash")

	currSchemaHash := models.SchemaHash{}
	res := db.Where("type = ? AND name = ?", hashType, name).Find(&currSchemaHash)
	if res.Error != nil {
		vlog.WithError(res.Error).Error("error looking up schema hash")
	}

	var updateRequired bool
	if currSchemaHash.ID == 0 {
		vlog.Debug("no current schema hash in db, creating")
		updateRequired = true
		currSchemaHash = models.SchemaHash{
			Type: string(hashType),
			Name: name,
			Hash: hashStr,
		}
	} else if currSchemaHash.Hash != hashStr {
		vlog.WithField("oldHash", currSchemaHash.Hash).Debug("schema hash has changed, recreating")
		currSchemaHash.Hash = hashStr
		updateRequired = true
	} else if forceUpdate {
		vlog.Debug("schema hash has not changed but a force update was requested, recreating")
		updateRequired = true
	}

	if updateRequired {
		if res := db.Exec(dropSQL); res.Error != nil {
			vlog.WithError(res.Error).Error("error dropping")
			return updateRequired, res.Error
		}

		vlog.Info("creating with latest schema")

		if res := db.Exec(desiredSchema); res.Error != nil {
			log.WithError(res.Error).Error("error creating")
			return updateRequired, res.Error
		}

		if currSchemaHash.ID == 0 {
			if res := db.Create(&currSchemaHash); res.Error != nil {
				vlog.WithError(res.Error).Error("error creating schema hash")
				return updateRequired, res.Error
			}
		} else {
			if res := db.Save(&currSchemaHash); res.Error != nil {
				vlog.WithError(res.Error).Error("error updating schema hash")
				return updateRequired, res.Error
			}
		}
		vlog.Info("schema hash updated")
	} else {
		vlog.Debug("no schema update required")
	}
	return updateRequired, nil
}

func ParseGormLogLevel(logLevel string) (gormlogger.LogLevel, error) {
	switch logLevel {
	case "info":
		return gormlogger.Info, nil
	case "warn":
		return gormlogger.Warn, nil
	case "error":
		return gormlogger.Error, nil
	case "silent":
		return gormlogger.Silent, nil
	default:
		return gormlogger.Info, fmt.Errorf("Unknown gorm LogLevel: %s", logLevel)
	}
}
