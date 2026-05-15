package partitionmanager

import (
	"context"
	"fmt"
	"regexp"
	"time"

	partman "github.com/jirevwe/go_partman"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var validIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const defaultSampleRate = 1 * time.Hour

type TableConfig struct {
	Name              string
	Schema            string
	PartitionColumn   string
	PartitionInterval time.Duration
	PartitionCount    uint
	RetentionPeriod   time.Duration
}

var DefaultTables = []TableConfig{
	{
		Name:              "test_analysis_by_job_by_dates",
		Schema:            "public",
		PartitionColumn:   "date",
		PartitionInterval: 24 * time.Hour,
		PartitionCount:    14,
		RetentionPeriod:   365 * 24 * time.Hour,
	},
}

type PartitionManager struct {
	manager *partman.Manager
}

func NewWithDefaults(gormDB *gorm.DB) (*PartitionManager, error) {
	return New(gormDB, defaultSampleRate, DefaultTables)
}

func New(gormDB *gorm.DB, sampleRate time.Duration, tables []TableConfig) (*PartitionManager, error) {
	if gormDB == nil {
		return nil, fmt.Errorf("gorm DB cannot be nil")
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get *sql.DB from GORM: %w", err)
	}

	sqlxDB := sqlx.NewDb(sqlDB, "postgres")

	partmanTables := make([]partman.Table, len(tables))
	for i, t := range tables {
		partmanTables[i] = partman.Table{
			Name:              t.Name,
			Schema:            t.Schema,
			PartitionBy:       t.PartitionColumn,
			PartitionType:     partman.TypeRange,
			PartitionInterval: t.PartitionInterval,
			PartitionCount:    t.PartitionCount,
			RetentionPeriod:   t.RetentionPeriod,
		}
	}

	cfg := &partman.Config{
		SampleRate: sampleRate,
		Tables:     partmanTables,
	}

	manager, err := partman.NewManager(
		partman.WithDB(sqlxDB),
		partman.WithConfig(cfg),
		partman.WithLogger(&logrusLogger{entry: log.WithField("source", "partman")}),
		partman.WithClock(partman.NewRealClock()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create partition manager: %w", err)
	}

	return &PartitionManager{manager: manager}, nil
}

func (pm *PartitionManager) Maintain(ctx context.Context) error {
	return pm.manager.Maintain(ctx)
}

func (pm *PartitionManager) Start(ctx context.Context) error {
	return pm.manager.Start(ctx)
}

func (pm *PartitionManager) Stop() {
	pm.manager.Stop()
}

// EnsurePartition creates a partition for a specific date on the given table
// if it doesn't already exist. go_partman only creates future partitions, so
// this covers historical dates needed during initial bulk loads.
// Uses YYYYMMDD naming to match go_partman's format for retention compatibility.
func EnsurePartition(db *gorm.DB, table, date, nextDay string) error {
	if !validIdentifier.MatchString(table) {
		return fmt.Errorf("invalid table name %q", table)
	}
	dateParsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return fmt.Errorf("invalid date %q: %w", date, err)
	}
	nextDayParsed, err := time.Parse("2006-01-02", nextDay)
	if err != nil {
		return fmt.Errorf("invalid nextDay %q: %w", nextDay, err)
	}
	partitionName := fmt.Sprintf("%s_%s", table, dateParsed.Format("20060102"))
	// DDL statements don't support parameterized placeholders; dates are
	// validated above via time.Parse so interpolation is safe.
	sql := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS "%s" PARTITION OF "%s" FOR VALUES FROM ('%s') TO ('%s')`,
		partitionName, table,
		dateParsed.Format("2006-01-02"), nextDayParsed.Format("2006-01-02"),
	)

	if res := db.Exec(sql); res.Error != nil {
		return fmt.Errorf("error creating partition %s: %w", partitionName, res.Error)
	}
	return nil
}

// logrusLogger bridges logrus to go_partman's Logger interface.
type logrusLogger struct {
	entry *log.Entry
}

func (l *logrusLogger) Info(args ...interface{})                  { l.entry.Info(args...) }
func (l *logrusLogger) Debug(args ...interface{})                 { l.entry.Debug(args...) }
func (l *logrusLogger) Warn(args ...interface{})                  { l.entry.Warn(args...) }
func (l *logrusLogger) Error(args ...interface{})                 { l.entry.Error(args...) }
func (l *logrusLogger) Fatal(args ...interface{})                 { l.entry.Fatal(args...) }
func (l *logrusLogger) Infof(format string, args ...interface{})  { l.entry.Infof(format, args...) }
func (l *logrusLogger) Debugf(format string, args ...interface{}) { l.entry.Debugf(format, args...) }
func (l *logrusLogger) Warnf(format string, args ...interface{})  { l.entry.Warnf(format, args...) }
func (l *logrusLogger) Errorf(format string, args ...interface{}) { l.entry.Errorf(format, args...) }
func (l *logrusLogger) Fatalf(format string, args ...interface{}) { l.entry.Fatalf(format, args...) }
