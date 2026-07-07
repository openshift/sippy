package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	log "github.com/sirupsen/logrus"
)

// PgxSession is satisfied by both *pgx.Conn and pgx.Tx.
type PgxSession interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

type TempColumn[T any] struct {
	Name  string
	Type  string
	Value func(*T) any
}

func CopyToTempTable[T any](
	ctx context.Context,
	conn PgxSession,
	tempTable string,
	rows []T,
	cols []TempColumn[T],
) (cleanup func(), err error) {
	if len(cols) == 0 {
		return func() {}, fmt.Errorf("CopyToTempTable: cols must not be empty")
	}

	drop := func() {
		if _, err := conn.Exec(context.Background(), "DROP TABLE IF EXISTS "+tempTable); err != nil {
			log.WithError(err).WithField("table", tempTable).Error("failed to drop temp table")
		}
	}

	colDefs := make([]string, len(cols))
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colDefs[i] = fmt.Sprintf("%s %s", c.Name, c.Type)
		colNames[i] = c.Name
	}

	createSQL := fmt.Sprintf("CREATE TEMP TABLE %s (%s)", tempTable, strings.Join(colDefs, ", "))
	if _, err := conn.Exec(ctx, createSQL); err != nil {
		return func() {}, fmt.Errorf("creating %s: %w", tempTable, err)
	}

	if _, err := conn.CopyFrom(ctx,
		pgx.Identifier{tempTable},
		colNames,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			vals := make([]any, len(cols))
			for j, c := range cols {
				vals[j] = c.Value(&rows[i])
			}
			return vals, nil
		}),
	); err != nil {
		drop()
		return func() {}, fmt.Errorf("COPY %s: %w", tempTable, err)
	}
	return drop, nil
}
