package flags

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db"
)

// Gorm Log Level Custom Flag Type
type logLevel logger.LogLevel

const (
	LogLevelInfo   = "info"
	LogLevelWarn   = "warn"
	LogLevelError  = "error"
	LogLevelSilent = "silent"
)

func (l *logLevel) String() string {
	switch *l {
	case logLevel(logger.Info):
		return LogLevelInfo
	case logLevel(logger.Warn):
		return LogLevelWarn
	case logLevel(logger.Error):
		return LogLevelError
	case logLevel(logger.Silent):
		return LogLevelSilent
	}

	return LogLevelInfo
}

func (l *logLevel) Set(v string) error {
	switch v {
	case LogLevelInfo:
		*l = logLevel(logger.Info)
	case LogLevelWarn:
		*l = logLevel(logger.Warn)
	case LogLevelError:
		*l = logLevel(logger.Error)
	case LogLevelSilent:
		*l = logLevel(logger.Silent)
	default:
		return fmt.Errorf("unknown gorm log level: %s", v)
	}

	return nil
}

func (l *logLevel) Type() string {
	return "logLevel"
}

// Date Time Custom Flag Type
type PinnedTime time.Time

func (p *PinnedTime) String() string {
	if time.Time(*p).IsZero() {
		return ""
	}

	return time.Time(*p).Format(time.RFC3339)
}

func (p *PinnedTime) Set(v string) error {
	parsedTime, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return err
	}

	*p = PinnedTime(parsedTime)
	return nil
}

func (p *PinnedTime) Type() string {
	return "pinnedTime"
}

func (f *PostgresFlags) GetPinnedTime() *time.Time {
	if time.Time(f.pinnedTime).IsZero() {
		return nil
	}

	t := time.Time(f.pinnedTime)
	return &t
}

// PostgresFlags contains the set of flags needed to connect to a postgres database.
type PostgresFlags struct {
	LogLevel logLevel
	DSN      string

	// pinnedTime should not be exported. Use GetPinnedTime() instead.
	pinnedTime PinnedTime
}

func NewPostgresDatabaseFlags(dsn string) *PostgresFlags {
	if dsn == "" {
		dsn = os.Getenv("SIPPY_DATABASE_DSN")
		if dsn == "" {
			dsn = "postgresql://postgres:password@localhost:5432/postgres"
		}
	}

	return &PostgresFlags{
		LogLevel: logLevel(logger.Info),
		DSN:      dsn,
	}
}

func (f *PostgresFlags) BindFlags(fs *pflag.FlagSet) {
	fs.Var(&f.LogLevel, "db-log-level", "GORM database log level")
	fs.StringVar(&f.DSN, "database-dsn", f.DSN, "Database DSN for connecting to Postgres")
	fs.Var(&f.pinnedTime, "pinned-date-time", "Pin database results to a fixed end date/time")
}

func (f *PostgresFlags) GetDBClient() (*db.DB, error) {
	dbc, err := db.New(f.DSN, logger.LogLevel(f.LogLevel))
	if err != nil {
		log.WithError(err).Error("could not connect to db")
		return nil, err
	}

	return dbc, nil
}
