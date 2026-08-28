package workqueue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	log "github.com/sirupsen/logrus"
)

// NewPgxV5Pool creates a pgx/v5 connection pool from a DSN. The returned pool
// is intended for River's exclusive use and coexists with the application's
// existing pgx/v4 pool.
func NewPgxV5Pool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating pgx/v5 pool: %w", err)
	}
	return pool, nil
}

// MigrateRiverSchema applies River's own schema migrations (the river_job table
// and supporting infrastructure). River manages its schema separately from
// Sippy's golang-migrate migrations. This is idempotent: already-applied
// versions are skipped.
func MigrateRiverSchema(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("creating River migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("running River migrations: %w", err)
	}
	log.Info("River schema migrations applied")
	return nil
}

// NewInsertOnlyClient creates a River client that can insert jobs but does not
// run workers. This is used by the API server, which creates batch
// specifications but does not process them.
func NewInsertOnlyClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("creating insert-only River client: %w", err)
	}
	return client, nil
}

// NewWorkerClient creates a fully configured River client that runs workers.
// The caller must register workers and configure queues in the provided config
// before calling Start() on the returned client.
func NewWorkerClient(pool *pgxpool.Pool, workers *river.Workers, config *river.Config) (*river.Client[pgx.Tx], error) {
	config.Workers = workers
	client, err := river.NewClient(riverpgxv5.New(pool), config)
	if err != nil {
		return nil, fmt.Errorf("creating worker River client: %w", err)
	}
	return client, nil
}
