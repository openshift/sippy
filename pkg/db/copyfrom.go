package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	log "github.com/sirupsen/logrus"
)

// CopyFrom uses the PostgreSQL COPY protocol to perform bulk data insertion,
// streaming all rows in a single protocol-level operation. Callers provide a
// pgx.CopyFromSource (e.g. pgx.CopyFromRows or pgx.CopyFromSlice) so they
// can choose whether to pre-allocate rows or generate them lazily.
//
// Columns not listed receive their DEFAULT (e.g. serial id, deleted_at NULL).
func (d *DB) CopyFrom(ctx context.Context, table string, columns []string, rowSrc pgx.CopyFromSource) (int64, error) {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return 0, fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return 0, fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	return conn.CopyFrom(ctx, pgx.Identifier{table}, columns, rowSrc)
}
