package db

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/openshift-eng/gopar/partitioning"
	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	sippymigrate "github.com/openshift/sippy/pkg/db/migrate"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

type SchemaHashType string

const (
	hashTypeMatView                          SchemaHashType = "matview"
	hashTypeView                             SchemaHashType = "view"
	hashTypeMatViewIndex                     SchemaHashType = "matview_index"
	hashTypeFunction                         SchemaHashType = "function"
	partitionedTableProwJobRunTests                         = "prow_job_run_tests"
	partitionedTableProwJobRunTestsOutputs                  = "prow_job_run_test_outputs"
	partitionedTableTestAnalysisByJobByDates                = "test_analysis_by_job_by_dates"
	partitionedTableTestDailyTotals                         = "test_daily_totals"
	partitionedTableTestCumulativeSummaries                 = "test_cumulative_summaries"
)

type DB struct {
	DB *gorm.DB

	// BatchSize is used for how many insertions we should do at once. Postgres supports
	// a maximum of 2^16 records per insert.
	BatchSize int

	// GoparPartitions provides partition creation/management operations
	GoparPartitions *partitioning.DB_PARTITIONS
}

// log2LogrusWriter bridges gorm logging to logrus logging.
// All messages will come through at DEBUG level.
type log2LogrusWriter struct {
	entry *log.Entry
}

func (w log2LogrusWriter) Printf(msg string, args ...any) {
	w.entry.Debugf(msg, args...)
}

type Option func(*options)

type options struct {
	enablePartitionwise bool
}

func WithPartitionwise(enable bool) Option {
	return func(o *options) {
		o.enablePartitionwise = enable
	}
}

func New(dsn string, logLevel gormlogger.LogLevel, opts ...Option) (*DB, error) {
	var cfg options
	for _, o := range opts {
		o(&cfg)
	}
	gormLogger := gormlogger.New(
		log2LogrusWriter{entry: log.WithField("source", "gorm")},
		gormlogger.Config{
			SlowThreshold:             2 * time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	pgxConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// Prevent PostgreSQL from generating generic plans for prepared statements.
	// With 10k+ partitions on tables like test_analysis_by_job_by_dates, generic
	// plan generation alone can take 17+ minutes as the planner enumerates all
	// partitions. Custom plans use actual parameter values for partition pruning.
	pgxConfig.RuntimeParams["plan_cache_mode"] = "force_custom_plan"
	pgxConfig.RuntimeParams["work_mem"] = "128MB"
	pgxConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "60s"
	pgxConfig.RuntimeParams["random_page_cost"] = "1.1"
	pgxConfig.RuntimeParams["timezone"] = "UTC"
	if cfg.enablePartitionwise {
		pgxConfig.RuntimeParams["enable_partitionwise_aggregate"] = "on"
		pgxConfig.RuntimeParams["enable_partitionwise_join"] = "on"
	}

	connPool := stdlib.OpenDB(*pgxConfig)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: connPool,
	}), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, err
	}

	// Get underlying sql.DB for gopar
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	return &DB{
		DB:              db,
		BatchSize:       1024,
		GoparPartitions: partitioning.NewPartitions(sqlDB),
	}, nil
}

func (d *DB) UpdateSchema(reportEnd *time.Time) error {

	// Run versioned migrations (golang-migrate) BEFORE AutoMigrate.
	// This ensures tables like prow_job_run_tests exist before
	// prow_job_runs trys to create it via AutoMigrate
	// when we move prow_job_runs to be managed via RunMigrations
	// we may need GORM AutoMigrate to run first
	if err := sippymigrate.RunMigrations(d.DB); err != nil {
		return err
	}

	// Register explicit join table so GORM uses our model (with release/timestamp)
	// instead of auto-generating a bare join table.
	if err := d.DB.SetupJoinTable(&models.ProwJobRun{}, "PullRequests", &models.ProwJobRunProwPullRequest{}); err != nil {
		return fmt.Errorf("setup join table ProwJobRun.PullRequests: %w", err)
	}

	// List of all models to migrate
	modelsToMigrate := []any{
		&models.ReleaseDefinition{},
		&models.ReleaseTag{},
		&models.ReleasePullRequest{},
		&models.ReleaseRepository{},
		&models.ReleaseJobRun{},
		&models.ProwGARawTestDatum{},
		&models.VariantCombination{},
		&models.ProwJob{},
		&models.ProwJobRun{},
		&models.ProwJobRunAnnotation{},
		&models.Test{},
		&models.Suite{},
		&models.APISnapshot{},
		&models.Bug{},
		&models.ProwPullRequest{},
		&models.ProwJobRunProwPullRequest{},
		&models.SchemaHash{},
		&models.PullRequestComment{},
		&models.JiraIncident{},
		&models.JiraComponent{},
		&models.TestOwnership{},
		&models.FeatureGate{},
		&models.TestRegression{},
		&models.RegressionJobRun{},
		&models.RegressionView{},
		&models.Triage{},
		&models.TriageSymptom{},
		&models.AuditLog{},
		&models.ChatRating{},
		&models.ChatConversation{},
		&jobrunscan.Label{},
		&jobrunscan.Symptom{},
		&models.TestDailySummary{},
	}

	// Currently we need RunMigrations to run prior
	// to AutoMigrate so that tables GORM depends on exist
	// prior to AutoMigrate
	// As we migrate more of the JobRuns based tables the
	// Dependencies change, and we likely need to run this first
	for _, model := range modelsToMigrate {
		if err := d.DB.AutoMigrate(model); err != nil {
			return err
		}
	}

	if err := createAuditLogIndexes(d.DB); err != nil {
		return err
	}

	if err := ensureTriageSymptomCascade(d.DB); err != nil {
		return err
	}

	if err := ensureVariantCombinationTrigger(d.DB); err != nil {
		return err
	}

	if err := populateTestSuitesInDB(d.DB); err != nil {
		return err
	}

	if err := syncPostgresMaterializedViews(d.DB, reportEnd); err != nil {
		return err
	}

	if err := syncPostgresViews(d.DB, reportEnd); err != nil {
		return err
	}

	return syncPostgresFunctions(d.DB)
}

// PartitionedTables returns the list of tables that are partitioned
// and managed by gopar partition lifecycle management.
func (d *DB) PartitionedTables() []string {
	return []string{
		partitionedTableProwJobRunTests,
		partitionedTableProwJobRunTestsOutputs,
		partitionedTableTestAnalysisByJobByDates,
		partitionedTableTestDailyTotals,
		partitionedTableTestCumulativeSummaries,
	}
}

// EnsurePartitions creates missing partitions for all managed partitioned tables.
// It uses LIST→RANGE nested partitioning where:
//   - Level 1: LIST partition by release (e.g., "4.17", "4.18")
//   - Level 2: RANGE sub-partition by timestamp (daily granularity)
//
// Parameters:
//   - releases: List of releases to create partitions for (e.g., ["4.17", "4.18", "4.19"])
//   - startDate: Start date for partition creation
//   - endDate: End date for partition creation
//   - dryRun: If true, only preview what would be created
//
// Returns the total number of partitions created across all tables.
func (d *DB) EnsurePartitions(releases []string, startDate, endDate time.Time, dryRun bool) (int, error) {
	totalCreated := 0

	for _, tableName := range d.PartitionedTables() {
		var dateColumn string
		switch tableName {
		case partitionedTableProwJobRunTests:
			dateColumn = "prow_job_run_timestamp"
		case partitionedTableProwJobRunTestsOutputs:
			dateColumn = "prow_job_run_test_timestamp"
		case partitionedTableTestAnalysisByJobByDates, partitionedTableTestDailyTotals, partitionedTableTestCumulativeSummaries:
			dateColumn = "date"
		default:
			log.Warnf("unknown partitioned table: %s", tableName)
			continue
		}

		log.Infof("Creating partitions for %s (releases: %v, dates: %s to %s)",
			tableName, releases, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

		count, err := d.GoparPartitions.CreateMissingPartitionsListToRange(
			tableName,
			releases,
			startDate,
			endDate,
			dateColumn,
			true, // usePartmanFormat - use partman-style partition naming
			dryRun,
		)
		if err != nil {
			return totalCreated, fmt.Errorf("failed to create partitions for %s: %w", tableName, err)
		}

		totalCreated += count
		log.Infof("Created %d partitions for %s", count, tableName)
	}

	return totalCreated, nil
}

// DetachOldPartitions detaches partitions older than the specified retention period.
// This is a safer alternative to immediate deletion - detached partitions can be
// archived or reviewed before permanent deletion.
//
// Parameters:
//   - retentionDays: Age threshold in days (e.g., 100 means detach partitions older than 100 days)
//   - dryRun: If true, only preview what would be detached
//
// Returns the total number of partitions detached across all tables.
func (d *DB) DetachOldPartitions(retentionDays int, dryRun bool) (int, error) {
	totalDetached := 0

	for _, tableName := range d.PartitionedTables() {
		log.Infof("Finding partitions to detach for %s (older than %d days)",
			tableName, retentionDays)

		// Get partitions that are attached and older than retention period
		partitions, err := d.GoparPartitions.GetPartitionsForRemoval(tableName, retentionDays, true)
		if err != nil {
			return totalDetached, fmt.Errorf("failed to get partitions for removal from %s: %w", tableName, err)
		}

		log.Infof("Found %d partitions to detach for %s", len(partitions), tableName)

		for _, partition := range partitions {
			if err := d.GoparPartitions.DetachPartition(partition.TableName, dryRun); err != nil {
				log.WithError(err).Errorf("failed to detach partition %s", partition.TableName)
				return totalDetached, fmt.Errorf("failed to detach partition %s: %w", partition.TableName, err)
			}
			totalDetached++
			if dryRun {
				log.Infof("[DRY RUN] Would detach partition %s", partition.TableName)
			} else {
				log.Infof("Detached partition %s", partition.TableName)
			}
		}
	}

	return totalDetached, nil
}

// DropDetachedPartitions drops partitions that have been detached for longer than
// the specified period. This permanently deletes the data.
//
// Parameters:
//   - detachedDays: Minimum age in days since detachment (e.g., 110 means drop partitions detached more than 110 days ago)
//   - dryRun: If true, only preview what would be dropped
//
// Returns the total number of partitions dropped across all tables.
func (d *DB) DropDetachedPartitions(detachedDays int, dryRun bool) (int, error) {
	totalDropped := 0

	for _, tableName := range d.PartitionedTables() {
		log.Infof("Finding detached partitions to drop for %s (detached more than %d days ago)",
			tableName, detachedDays)

		// Get partitions that are detached and older than detached period
		partitions, err := d.GoparPartitions.GetPartitionsForRemoval(tableName, detachedDays, false)
		if err != nil {
			return totalDropped, fmt.Errorf("failed to get detached partitions for removal from %s: %w", tableName, err)
		}

		log.Infof("Found %d detached partitions to drop for %s", len(partitions), tableName)

		for _, partition := range partitions {
			if err := d.GoparPartitions.DropPartition(partition.TableName, dryRun); err != nil {
				log.WithError(err).Errorf("failed to drop partition %s", partition.TableName)
				return totalDropped, fmt.Errorf("failed to drop partition %s: %w", partition.TableName, err)
			}
			totalDropped++
			if dryRun {
				log.Infof("[DRY RUN] Would drop partition %s", partition.TableName)
			} else {
				log.Infof("Dropped partition %s", partition.TableName)
			}
		}
	}

	return totalDropped, nil
}

// CleanupPartitions performs the full partition lifecycle cleanup:
// 1. Detaches partitions older than 100 days
// 2. Drops detached partitions older than 110 days
//
// This provides a 10-day safety window between detachment and permanent deletion.
//
// Parameters:
//   - dryRun: If true, only preview what would be done
//
// Returns the number of partitions detached and dropped.
func (d *DB) CleanupPartitions(dryRun bool) (detached, dropped int, err error) {
	log.Info("Starting partition cleanup...")

	// First, drop old detached partitions (110 days)
	dropped, err = d.DropDetachedPartitions(110, dryRun)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to drop detached partitions: %w", err)
	}
	log.Infof("Dropped %d detached partitions", dropped)

	// Then, detach old attached partitions (100 days)
	detached, err = d.DetachOldPartitions(100, dryRun)
	if err != nil {
		return detached, dropped, fmt.Errorf("failed to detach old partitions: %w", err)
	}
	log.Infof("Detached %d old partitions", detached)

	log.Infof("Partition cleanup complete: detached=%d, dropped=%d", detached, dropped)
	return detached, dropped, nil
}

// syncSchema will update generic db resources if their schema has changed. (functions, materialized views, indexes)
// This is useful for resources that cannot be updated incrementally with goose, and can cause conflict / last write
// wins problems with concurrent development.
//
// desiredSchema should be the full SQL command we would issue to create the resource fresh. It will be hashed and
// compared to a pre-existing value in the db of the given name and type, if any exists. If none exists, or the hashes
// have changed, the resource will be recreated. If the hash matches but the resource is missing from the database
// (e.g. dropped externally), it will also be recreated.
//
// dropSQL is the full SQL command we will run if we detect that the resource needs updating. It should include
// "IF EXISTS" as it will be attempted even when no previous resource exists. (i.e. new databases)
//
// returns true if the resource was recreated
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
	switch {
	case currSchemaHash.ID == 0:
		vlog.Debug("no current schema hash in db, creating")
		updateRequired = true
		currSchemaHash = models.SchemaHash{
			Type: string(hashType),
			Name: name,
			Hash: hashStr,
		}
	case currSchemaHash.Hash != hashStr:
		vlog.WithField("oldHash", currSchemaHash.Hash).Debug("schema hash has changed, recreating")
		currSchemaHash.Hash = hashStr
		updateRequired = true
	case forceUpdate:
		vlog.Debug("schema hash has not changed but a force update was requested, recreating")
		updateRequired = true
	default:
		exists, err := resourceExists(db, hashType, name)
		if err != nil {
			return false, err
		}
		if !exists {
			vlog.Warn("schema hash matches but resource is missing from database, recreating")
			updateRequired = true
		}
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

// resourceExists checks whether a database resource actually exists by querying
// the PostgreSQL system catalogs. This catches cases where a resource was dropped
// externally but its schema_hashes record remained.
func resourceExists(db *gorm.DB, hashType SchemaHashType, name string) (bool, error) {
	var query string
	switch hashType {
	case hashTypeMatView:
		query = "SELECT 1 FROM pg_class WHERE relname = ? AND relkind = 'm'"
	case hashTypeView:
		query = "SELECT 1 FROM pg_class WHERE relname = ? AND relkind = 'v'"
	case hashTypeMatViewIndex:
		query = "SELECT 1 FROM pg_class WHERE relname = ? AND relkind = 'i'"
	case hashTypeFunction:
		query = "SELECT 1 FROM pg_proc WHERE proname = ?"
	default:
		return false, fmt.Errorf("unknown schema hash type: %s", hashType)
	}
	var exists int
	if res := db.Raw(query, name).Scan(&exists); res.Error != nil || res.RowsAffected == 0 {
		return false, nil
	}
	return true, nil
}

// createAuditLogIndexes creates GIN indexes for JSONB columns in audit_logs table
// for efficient JSON querying operations.
func createAuditLogIndexes(db *gorm.DB) error {
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_new_data_gin ON audit_logs USING GIN (new_data)").Error; err != nil {
		return fmt.Errorf("failed to create GIN index on audit_logs.new_data: %w", err)
	}

	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_old_data_gin ON audit_logs USING GIN (old_data)").Error; err != nil {
		return fmt.Errorf("failed to create GIN index on audit_logs.old_data: %w", err)
	}

	return nil
}

// ensureTriageSymptomCascade adds foreign keys to triage_symptoms with ON DELETE CASCADE
// so that deleting a symptom definition or a regression automatically cleans up the
// associated triage_symptoms rows.
func ensureTriageSymptomCascade(db *gorm.DB) error {
	constraints := []struct {
		name string
		sql  string
	}{
		{
			name: "fk_triage_symptoms_symptom",
			sql:  "ALTER TABLE triage_symptoms ADD CONSTRAINT fk_triage_symptoms_symptom FOREIGN KEY (symptom_id) REFERENCES job_run_symptoms(id) ON DELETE CASCADE",
		},
		{
			name: "fk_triage_symptoms_regression",
			sql:  "ALTER TABLE triage_symptoms ADD CONSTRAINT fk_triage_symptoms_regression FOREIGN KEY (regression_id) REFERENCES test_regressions(id) ON DELETE CASCADE",
		},
	}

	for _, c := range constraints {
		err := db.Exec(fmt.Sprintf(`
			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint
					WHERE conname = '%s'
				) THEN
					%s;
				END IF;
			END $$`, c.name, c.sql)).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// ensureVariantCombinationTrigger attaches the variant_combination_id
// trigger to prow_jobs if it does not already exist. The trigger
// function is created by migration 000003; the table is created by
// AutoMigrate, so this must run after both.
//
// When the trigger is first attached, existing rows are backfilled.
// In steady state (trigger already exists) this is a single catalog
// lookup.
func ensureVariantCombinationTrigger(db *gorm.DB) error {
	return db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_trigger WHERE tgname = 'trg_prow_jobs_variant_combination'
			) THEN
				CREATE TRIGGER trg_prow_jobs_variant_combination
					BEFORE INSERT OR UPDATE OF variants ON prow_jobs
					FOR EACH ROW
					EXECUTE FUNCTION set_variant_combination_id();

				-- Backfill only needed when trigger is first attached (fresh DB
				-- or recovery); the trigger handles all subsequent rows.
				INSERT INTO variant_combinations (variants)
				SELECT DISTINCT variants FROM prow_jobs
				WHERE variants IS NOT NULL AND variant_combination_id IS NULL
				ON CONFLICT (variants) DO NOTHING;

				UPDATE prow_jobs
				SET variant_combination_id = vc.id
				FROM variant_combinations vc
				WHERE prow_jobs.variants = vc.variants
				  AND prow_jobs.variants IS NOT NULL
				  AND prow_jobs.variant_combination_id IS NULL;
			END IF;
		END $$`).Error
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
		return gormlogger.Info, fmt.Errorf("unknown gorm LogLevel: %s", logLevel)
	}
}
