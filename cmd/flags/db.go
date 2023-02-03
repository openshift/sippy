package flags

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"
	"gorm.io/gorm/logger"
)

// Gorm Log Level Custom Flag Type
type logLevel logger.LogLevel

func (l *logLevel) String() string {
	switch *l {
	case logLevel(logger.Info):
		return "info"
	case logLevel(logger.Warn):
		return "warn"
	case logLevel(logger.Error):
		return "error"
	case logLevel(logger.Silent):
		return "silent"
	}

	return "info"
}

func (l *logLevel) Set(v string) error {
	switch v {
	case "info":
		*l = logLevel(logger.Info)
	case "warn":
		*l = logLevel(logger.Warn)
	case "error":
		*l = logLevel(logger.Error)
	case "silent":
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
type pinnedTime time.Time

func (p *pinnedTime) String() string {
	return time.Time(*p).Format(time.RFC3339)
}

func (p *pinnedTime) Set(v string) error {
	parsedTime, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return err
	}

	*p = pinnedTime(parsedTime)
	return nil
}

func (p *pinnedTime) Type() string {
	return "pinnedTime"
}

// PostgresDatabaseFlags contains the set of flags needed to connect to a postgres database.
type PostgresDatabaseFlags struct {
	LogLevel   logLevel
	PinnedTime pinnedTime
	DSN        string
}

func NewPostgresDatabaseFlags() *PostgresDatabaseFlags {
	dsn := os.Getenv("SIPPY_DATABASE_DSN")
	if dsn == "" {
		dsn = "postgresql://postgres:password@localhost:5432/postgres"
	}

	return &PostgresDatabaseFlags{
		LogLevel:   logLevel(logger.Info),
		DSN:        dsn,
		PinnedTime: pinnedTime(time.Now()),
	}
}

func (f *PostgresDatabaseFlags) BindFlags(fs *pflag.FlagSet) {
	fs.Var(&f.LogLevel, "database-log-level", "gorm database log level")
	fs.StringVar(&f.DSN, "database-dsn", f.DSN, "Database DSN for connecting to Postgres")
	fs.Var(&f.PinnedTime, "pinned-date-time", "optional value to use in a historical context with a fixed date/time")
}
