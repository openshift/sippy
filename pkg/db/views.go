package db

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

const replaceTimeNow = "|||TIMENOW|||"
const timestampFormat = "2006-01-02 15:04:05"

// TODO: for historical sippy we need to specify the pinnedDate and not use NOW
var PostgresMatViews = []PostgresView{}

// PostgresViews are regular, non-materialized views:
var PostgresViews = []PostgresView{}

type PostgresView struct {
	// Name is the name of the materialized view in postgres.
	Name string
	// Definition is the material view definition.
	Definition string
	// ReplaceStrings is a map of strings we want to replace in the create view statement, allowing for re-use.
	ReplaceStrings map[string]string
	// IndexColumns are the columns to create a unique index for. Will be named idx_[Name] and automatically
	// replaced if changes are made to these values. IndexColumns are required as we need them defined to be able to
	// refresh materialized views concurrently. (avoiding locking reads for several minutes while we update)
	IndexColumns []string
	// AdditionalIndexes are non-unique indexes to create on the materialized view for query performance.
	// Each entry is a raw column expression (e.g. "release, timestamp DESC") and will be named
	// idx_[Name]_[sequence].
	AdditionalIndexes []string
	// RefreshPhase controls the order in which matviews are refreshed. All matviews
	// in phase 0 refresh first (concurrently), then all in phase 1, and so on.
	// Use this when a matview reads from another matview and needs it to be up-to-date.
	// The default zero value means phase 0.
	RefreshPhase int
}

// RefreshByPhase groups matviews by RefreshPhase and calls refreshFn for each
// phase in order. All matviews in a phase are passed to refreshFn together
// (the caller is responsible for concurrent execution within a phase).
func RefreshByPhase(matviews []PostgresView, refreshFn func([]PostgresView)) {
	sorted := make([]PostgresView, len(matviews))
	copy(sorted, matviews)
	slices.SortFunc(sorted, func(a, b PostgresView) int {
		return a.RefreshPhase - b.RefreshPhase
	})

	for i := 0; i < len(sorted); {
		phase := sorted[i].RefreshPhase
		j := i
		for j < len(sorted) && sorted[j].RefreshPhase == phase {
			j++
		}
		refreshFn(sorted[i:j])
		i = j
	}
}

func syncPostgresMaterializedViews(db *gorm.DB, reportEnd *time.Time) error {

	// initialize outside our loop
	reportEndFmt := "NOW()"

	if reportEnd != nil {
		reportEndFmt = "TO_TIMESTAMP('" + reportEnd.UTC().Format(timestampFormat) + "', 'YYYY-MM-DD HH24:MI:SS')"
	}

	for _, pmv := range PostgresMatViews {
		// Sync materialized view:
		viewDef := pmv.Definition
		for k, v := range pmv.ReplaceStrings {
			viewDef = strings.ReplaceAll(viewDef, k, v)
		}

		// This has to occur after the replaceAll above as they might contain the REPLACE_TIME_NOW constant as well
		viewDef = strings.ReplaceAll(viewDef, replaceTimeNow, reportEndFmt)

		// CASCADE is safe here: dependent matviews (e.g. collapsed matviews) are
		// all in PostgresMatViews and will be detected as missing and recreated
		// by this same sync loop.
		dropSQL := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s CASCADE", pmv.Name)
		schema := fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS %s WITH NO DATA", pmv.Name, viewDef)
		matViewUpdated, err := syncSchema(db, hashTypeMatView, pmv.Name, schema, dropSQL, false)
		if err != nil {
			return err
		}

		// Sync index for the materialized view:
		indexName := fmt.Sprintf("idx_%s", pmv.Name)
		index := fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s(%s)", indexName, pmv.Name, strings.Join(pmv.IndexColumns, ","))
		dropSQL = fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
		if _, err := syncSchema(db, hashTypeMatViewIndex, indexName, index, dropSQL, matViewUpdated); err != nil {
			return err
		}

		for i, cols := range pmv.AdditionalIndexes {
			idxName := fmt.Sprintf("idx_%s_%d", pmv.Name, i)
			idxSQL := fmt.Sprintf("CREATE INDEX %s ON %s(%s)", idxName, pmv.Name, cols)
			dropSQL = fmt.Sprintf("DROP INDEX IF EXISTS %s", idxName)
			if _, err := syncSchema(db, hashTypeMatViewIndex, idxName, idxSQL, dropSQL, matViewUpdated); err != nil {
				return err
			}
		}
	}

	return nil
}

func syncPostgresViews(db *gorm.DB, reportEnd *time.Time) error {

	// initialize outside our loop
	reportEndFmt := "NOW()"

	if reportEnd != nil {
		reportEndFmt = "TO_TIMESTAMP('" + reportEnd.UTC().Format(timestampFormat) + "', 'YYYY-MM-DD HH24:MI:SS')"
	}

	for _, pmv := range PostgresViews {
		// Sync view:
		viewDef := pmv.Definition
		for k, v := range pmv.ReplaceStrings {
			viewDef = strings.ReplaceAll(viewDef, k, v)
		}

		// This has to occur after the replaceAll above as they might contain the REPLACE_TIME_NOW constant as well
		viewDef = strings.ReplaceAll(viewDef, replaceTimeNow, reportEndFmt)

		dropSQL := fmt.Sprintf("DROP VIEW IF EXISTS %s", pmv.Name)
		schema := fmt.Sprintf("CREATE OR REPLACE VIEW %s AS %s", pmv.Name, viewDef)
		_, err := syncSchema(db, hashTypeView, pmv.Name, schema, dropSQL, false)
		if err != nil {
			return err
		}
	}

	return nil
}
