package workqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	log "github.com/sirupsen/logrus"
)

// SetupConfig holds the configuration for setting up River.
type SetupConfig struct {
	DatabaseDSN           string
	Queues                map[string]river.QueueConfig
	Workers               *river.Workers
	CompletedJobRetention time.Duration
	DiscardedJobRetention time.Duration
}

// SetupResult holds the outputs of a successful River setup.
type SetupResult struct {
	Pool   *pgxpool.Pool
	Client *river.Client[pgx.Tx]
}

// Setup creates a pgx/v5 pool, runs River migrations, and returns a configured
// River client. If Queues is empty, the client operates in insert-only mode
// (no workers started).
func Setup(ctx context.Context, cfg SetupConfig) (*SetupResult, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("creating pgx/v5 pool: %w", err)
	}

	driver := riverpgxv5.New(pool)

	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("creating River migrator: %w", err)
	}

	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("running River migrations: %w", err)
	}
	if len(res.Versions) > 0 {
		log.WithField("versions", len(res.Versions)).Info("River migrations applied")
	}

	riverConfig := &river.Config{
		Workers: cfg.Workers,
	}
	if len(cfg.Queues) > 0 {
		riverConfig.Queues = cfg.Queues
	}
	if cfg.CompletedJobRetention > 0 {
		riverConfig.CompletedJobRetentionPeriod = cfg.CompletedJobRetention
	}
	if cfg.DiscardedJobRetention > 0 {
		riverConfig.DiscardedJobRetentionPeriod = cfg.DiscardedJobRetention
	}

	client, err := river.NewClient(driver, riverConfig)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("creating River client: %w", err)
	}

	return &SetupResult{Pool: pool, Client: client}, nil
}
