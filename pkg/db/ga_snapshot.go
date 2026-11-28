package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrGARawWindowIncomplete indicates that the retained raw partition tree
// does not cover a GA window completely.
var ErrGARawWindowIncomplete = errors.New("incomplete GA raw window")

// CheckGARawWindow verifies that attached run and test partitions cover the
// entire UTC interval. It does not establish whether ingestion was complete.
func CheckGARawWindow(tx *gorm.DB, release string, start, end time.Time) error {
	for _, table := range []string{"prow_job_runs", "prow_job_run_tests"} {
		if err := ensureGATableWindow(tx, table, release, start, end); err != nil {
			return err
		}
	}
	return nil
}

type partitionRange struct {
	Start time.Time
	End   time.Time
}

// ensureGATableWindow checks catalog bounds, including empty but attached partitions.
// Missing or detached partitions must never be mistaken for an empty snapshot.
func ensureGATableWindow(tx *gorm.DB, table, release string, start, end time.Time) error {
	var ranges []partitionRange
	err := tx.Raw(`
        SELECT bounds[1]::timestamptz AS start, bounds[2]::timestamptz AS "end"
        FROM pg_class root
        JOIN pg_inherits ri ON ri.inhparent = root.oid
        JOIN pg_class release_partition ON release_partition.oid = ri.inhrelid
        JOIN pg_inherits di ON di.inhparent = release_partition.oid
        JOIN pg_class daily ON daily.oid = di.inhrelid
        CROSS JOIN LATERAL regexp_match(pg_get_expr(daily.relpartbound, daily.oid),
             $re$FROM \('([^']+)'\) TO \('([^']+)'\)$re$) AS matches(bounds)
        WHERE root.oid = ?::regclass
          AND NOT ri.inhdetachpending AND NOT di.inhdetachpending
          AND pg_get_expr(release_partition.relpartbound, release_partition.oid) = format('FOR VALUES IN (%L)', ?::text)
        ORDER BY 1`, table, release).Scan(&ranges).Error
	if err != nil {
		return fmt.Errorf("checking %s partitions: %w", table, err)
	}
	covered, complete := gaWindowCoverageEnd(ranges, start, end)
	if complete {
		return nil
	}
	return fmt.Errorf("%w: %s partitions for %s at %s", ErrGARawWindowIncomplete, table, release, covered.UTC().Format(time.RFC3339))
}

// gaWindowCoverageEnd returns the last covered point and whether the sorted
// partition ranges fully cover the requested window.
func gaWindowCoverageEnd(ranges []partitionRange, start, end time.Time) (time.Time, bool) {
	covered := start
	for _, r := range ranges {
		if !r.End.After(covered) {
			continue
		}
		if r.Start.After(covered) {
			break
		}
		covered = r.End
		if !covered.Before(end) {
			return covered, true
		}
	}
	return covered, false
}
