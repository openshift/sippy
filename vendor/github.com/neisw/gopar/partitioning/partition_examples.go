package partitioning

import (
	"database/sql"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

// ExampleListPartitionedTables connects to the database and prints every
// partitioned table along with its strategy and current partition count.
func ExampleListPartitionedTables(db *sql.DB) error {
	dbp := NewPartitions(db)

	tables, err := dbp.ListPartitionedTables()
	if err != nil {
		return fmt.Errorf("listing partitioned tables: %w", err)
	}

	log.Infof("found %d partitioned table(s)", len(tables))
	for _, t := range tables {
		log.WithFields(log.Fields{
			"table":      t.TableName,
			"schema":     t.SchemaName,
			"strategy":   t.PartitionStrategy,
			"partitions": t.PartitionCount,
		}).Info("partitioned table")
	}

	return nil
}

// ExampleAddRelease creates a new LIST partition member (release) under a
// LIST → RANGE nested table, then populates it with daily sub-partitions for
// the given date range.
//
// Table structure after the call:
//
//	events (LIST by release_name)
//	└── events_v4_0 (RANGE by event_date)
//	    ├── events_v4_0_2024_01_01
//	    ├── events_v4_0_2024_01_02
//	    └── ...
func ExampleAddRelease(db *sql.DB, tableName, release, dateColumn string, start, end time.Time, dryRun bool) error {
	dbp := NewPartitions(db)

	created, err := dbp.CreateMissingPartitionsListToRange(
		tableName,
		[]string{release},
		start,
		end,
		dateColumn,
		false, // usePartmanFormat
		dryRun,
	)
	if err != nil {
		return fmt.Errorf("creating release %s partitions: %w", release, err)
	}

	log.WithFields(log.Fields{
		"table":   tableName,
		"release": release,
		"created": created,
		"dry_run": dryRun,
	}).Info("add release complete")

	return nil
}

// ExampleDetachRelease detaches an entire release partition (the intermediate
// LIST member and all of its daily children) from a nested partitioned table.
// The detached tables remain on disk until explicitly dropped.
func ExampleDetachRelease(db *sql.DB, tableName, release string, dryRun bool) error {
	dbp := NewPartitions(db)

	safeName := sanitizePartitionName(release)
	intermediatePartition := fmt.Sprintf("%s_%s", tableName, safeName)

	attached, err := dbp.IsPartitionAttached(intermediatePartition)
	if err != nil {
		return fmt.Errorf("checking attachment for %s: %w", intermediatePartition, err)
	}

	if !attached {
		log.WithField("partition", intermediatePartition).Info("partition is already detached")
		return nil
	}

	if err := dbp.DetachPartition(intermediatePartition, dryRun); err != nil {
		return fmt.Errorf("detaching release partition %s: %w", intermediatePartition, err)
	}

	log.WithFields(log.Fields{
		"table":     tableName,
		"release":   release,
		"partition": intermediatePartition,
		"dry_run":   dryRun,
	}).Info("detach release complete")

	return nil
}

// ExampleAddDailyPartitions creates daily RANGE partitions that are missing for
// a single-level RANGE-partitioned table over the given date range.
func ExampleAddDailyPartitions(db *sql.DB, tableName string, start, end time.Time, dryRun bool) error {
	dbp := NewPartitions(db)

	created, err := dbp.CreateMissingPartitions(tableName, start, end, false, dryRun)
	if err != nil {
		return fmt.Errorf("creating daily partitions: %w", err)
	}

	log.WithFields(log.Fields{
		"table":   tableName,
		"start":   start.Format("2006-01-02"),
		"end":     end.Format("2006-01-02"),
		"created": created,
		"dry_run": dryRun,
	}).Info("add daily partitions complete")

	return nil
}

// ExampleDetachOldDailyPartitions detaches daily partitions older than
// retentionDays from a RANGE-partitioned table.  Detached partitions stay on
// disk as standalone tables until they are explicitly dropped.
func ExampleDetachOldDailyPartitions(db *sql.DB, tableName string, retentionDays int, dryRun bool) error {
	dbp := NewPartitions(db)

	detached, err := dbp.DetachOldPartitions(tableName, retentionDays, dryRun)
	if err != nil {
		return fmt.Errorf("detaching old partitions: %w", err)
	}

	log.WithFields(log.Fields{
		"table":          tableName,
		"retention_days": retentionDays,
		"detached":       detached,
		"dry_run":        dryRun,
	}).Info("detach old daily partitions complete")

	return nil
}

// ExampleDropDetachedPartitions permanently removes detached partitions that
// are older than retentionDays.  This is destructive — data is deleted from
// disk.  Call ExampleDetachOldDailyPartitions (or ExampleDetachRelease) first
// so the partitions are detached before dropping.
func ExampleDropDetachedPartitions(db *sql.DB, tableName string, retentionDays int, dryRun bool) error {
	dbp := NewPartitions(db)

	detached, err := dbp.ListDetachedPartitions(tableName)
	if err != nil {
		return fmt.Errorf("listing detached partitions: %w", err)
	}

	log.WithFields(log.Fields{
		"table":    tableName,
		"detached": len(detached),
	}).Info("detached partitions before drop")
	for _, p := range detached {
		log.WithFields(log.Fields{
			"partition": p.TableName,
			"date":      p.PartitionDate.Format("2006-01-02"),
			"age_days":  p.Age,
			"size":      p.SizePretty,
		}).Info("  detached partition")
	}

	dropped, err := dbp.DropOldDetachedPartitions(tableName, retentionDays, dryRun)
	if err != nil {
		return fmt.Errorf("dropping detached partitions: %w", err)
	}

	log.WithFields(log.Fields{
		"table":          tableName,
		"retention_days": retentionDays,
		"dropped":        dropped,
		"dry_run":        dryRun,
	}).Info("drop detached partitions complete")

	return nil
}

// ExampleFullLifecycle demonstrates a complete partition management workflow
// for a LIST → RANGE nested table (e.g., partitioned by release then by date):
//
//  1. List all partitioned tables
//  2. Add a new release with daily sub-partitions
//  3. Detach an old release
//  4. Add new daily partitions for an existing release
//  5. Detach old daily partitions from a release
//  6. Drop all detached partitions past the retention window
func ExampleFullLifecycle(db *sql.DB, tableName, dateColumn string, dryRun bool) error {

	// ── 1. List partitioned tables ──────────────────────────────────────
	if err := ExampleListPartitionedTables(db); err != nil {
		return err
	}

	// ── 2. Add a new release ────────────────────────────────────────────
	newRelease := "v4.19"
	partitionStart := time.Now().Truncate(24 * time.Hour)
	partitionEnd := partitionStart.AddDate(0, 3, 0) // 3 months of daily partitions

	if err := ExampleAddRelease(db, tableName, newRelease, dateColumn, partitionStart, partitionEnd, dryRun); err != nil {
		return err
	}

	// ── 3. Detach an old release ────────────────────────────────────────
	oldRelease := "v4.16"

	if err := ExampleDetachRelease(db, tableName, oldRelease, dryRun); err != nil {
		return err
	}

	// ── 4. Add new daily partitions for the current release ─────────────
	// Extend an existing release's date range forward by 30 days.
	extendStart := partitionEnd.AddDate(0, 0, 1)
	extendEnd := extendStart.AddDate(0, 0, 30)

	intermediatePartition := fmt.Sprintf("%s_%s", tableName, sanitizePartitionName(newRelease))
	if err := ExampleAddDailyPartitions(db, intermediatePartition, extendStart, extendEnd, dryRun); err != nil {
		return err
	}

	// ── 5. Detach old daily partitions within a release ─────────────────
	retentionDays := 90
	activeRelease := "v4.18"
	activeIntermediate := fmt.Sprintf("%s_%s", tableName, sanitizePartitionName(activeRelease))

	if err := ExampleDetachOldDailyPartitions(db, activeIntermediate, retentionDays, dryRun); err != nil {
		return err
	}

	// ── 6. Drop detached partitions ─────────────────────────────────────
	if err := ExampleDropDetachedPartitions(db, tableName, retentionDays, dryRun); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"table":   tableName,
		"dry_run": dryRun,
	}).Info("full lifecycle example complete")

	return nil
}
