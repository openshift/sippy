package partitions

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

// PartitionInfo holds metadata about a partition
type PartitionInfo struct {
	TableName     string    `gorm:"column:tablename"`
	SchemaName    string    `gorm:"column:schemaname"`
	PartitionDate time.Time `gorm:"column:partition_date"`
	Age           int       `gorm:"column:age_days"`
	SizeBytes     int64     `gorm:"column:size_bytes"`
	SizePretty    string    `gorm:"column:size_pretty"`
	RowEstimate   int64     `gorm:"column:row_estimate"`
}

// PartitionedTableInfo holds metadata about a partitioned parent table
type PartitionedTableInfo struct {
	TableName         string `gorm:"column:tablename"`
	SchemaName        string `gorm:"column:schemaname"`
	PartitionCount    int    `gorm:"column:partition_count"`
	PartitionStrategy string `gorm:"column:partition_strategy"`
}

// PartitionStats holds aggregate statistics about partitions
type PartitionStats struct {
	TotalPartitions int
	TotalSizeBytes  int64
	TotalSizePretty string
	OldestDate      time.Time
	NewestDate      time.Time
	AvgSizeBytes    int64
	AvgSizePretty   string
}

// RetentionSummary provides a summary of what would be affected by a retention policy
type RetentionSummary struct {
	RetentionDays      int
	CutoffDate         time.Time
	PartitionsToRemove int
	StorageToReclaim   int64
	StoragePretty      string
	OldestPartition    string
	NewestPartition    string
}

// ListPartitionedTables returns all partitioned parent tables in the database
func ListPartitionedTables(dbc *db.DB) ([]PartitionedTableInfo, error) {
	start := time.Now()
	var tables []PartitionedTableInfo

	query := `
		SELECT
			c.relname AS tablename,
			n.nspname AS schemaname,
			COUNT(i.inhrelid)::INT AS partition_count,
			CASE pp.partstrat
				WHEN 'r' THEN 'RANGE'
				WHEN 'l' THEN 'LIST'
				WHEN 'h' THEN 'HASH'
				ELSE 'UNKNOWN'
			END AS partition_strategy
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_partitioned_table pp ON pp.partrelid = c.oid
		LEFT JOIN pg_inherits i ON i.inhparent = c.oid
		WHERE n.nspname = 'public'
		GROUP BY c.relname, n.nspname, pp.partstrat
		ORDER BY c.relname
	`

	result := dbc.DB.Raw(query).Scan(&tables)
	if result.Error != nil {
		log.WithError(result.Error).Error("failed to list partitioned tables")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"count":   len(tables),
		"elapsed": elapsed,
	}).Info("listed partitioned tables")

	return tables, nil
}

// ListTablePartitions returns all partitions for a given table
func ListTablePartitions(dbc *db.DB, tableName string) ([]PartitionInfo, error) {
	start := time.Now()
	var partitions []PartitionInfo

	query := `
		SELECT
			tablename,
			'public' as schemaname,
			TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
			(CURRENT_DATE - TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD'))::INT AS age_days,
			pg_total_relation_size('public.'||tablename) AS size_bytes,
			pg_size_pretty(pg_total_relation_size('public.'||tablename)) AS size_pretty,
			COALESCE(n_live_tup, 0) AS row_estimate
		FROM pg_tables
		LEFT JOIN pg_stat_user_tables ON pg_stat_user_tables.relname = pg_tables.tablename
			AND pg_stat_user_tables.schemaname = pg_tables.schemaname
		WHERE pg_tables.schemaname = 'public'
			AND pg_tables.tablename LIKE @table_pattern
		ORDER BY partition_date ASC
	`

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query, sql.Named("table_pattern", tablePattern)).Scan(&partitions)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to list table partitions")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":   tableName,
		"count":   len(partitions),
		"elapsed": elapsed,
	}).Info("listed table partitions")

	return partitions, nil
}

// GetPartitionStats returns aggregate statistics about partitions for a given table
func GetPartitionStats(dbc *db.DB, tableName string) (*PartitionStats, error) {
	start := time.Now()
	var stats PartitionStats

	query := `
		WITH partition_info AS (
			SELECT
				tablename,
				TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
				pg_total_relation_size('public.'||tablename) AS size_bytes
			FROM pg_tables
			WHERE schemaname = 'public'
				AND tablename LIKE @table_pattern
		)
		SELECT
			COUNT(*)::INT AS total_partitions,
			SUM(size_bytes)::BIGINT AS total_size_bytes,
			pg_size_pretty(SUM(size_bytes)) AS total_size_pretty,
			MIN(partition_date) AS oldest_date,
			MAX(partition_date) AS newest_date,
			AVG(size_bytes)::BIGINT AS avg_size_bytes,
			pg_size_pretty(AVG(size_bytes)::BIGINT) AS avg_size_pretty
		FROM partition_info
	`

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query, sql.Named("table_pattern", tablePattern)).Scan(&stats)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to get partition statistics")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":            tableName,
		"total_partitions": stats.TotalPartitions,
		"total_size":       stats.TotalSizePretty,
		"elapsed":          elapsed,
	}).Info("retrieved partition statistics")

	return &stats, nil
}

// GetPartitionsForRemoval identifies partitions older than the retention period for a given table
// This is a read-only operation (dry-run) that shows what would be removed (deleted or detached)
func GetPartitionsForRemoval(dbc *db.DB, tableName string, retentionDays int) ([]PartitionInfo, error) {
	start := time.Now()
	var partitions []PartitionInfo

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	query := `
		SELECT
			tablename,
			'public' as schemaname,
			TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
			(CURRENT_DATE - TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD'))::INT AS age_days,
			pg_total_relation_size('public.'||tablename) AS size_bytes,
			pg_size_pretty(pg_total_relation_size('public.'||tablename)) AS size_pretty,
			COALESCE(n_live_tup, 0) AS row_estimate
		FROM pg_tables
		LEFT JOIN pg_stat_user_tables ON pg_stat_user_tables.relname = pg_tables.tablename
			AND pg_stat_user_tables.schemaname = pg_tables.schemaname
		WHERE pg_tables.schemaname = 'public'
			AND pg_tables.tablename LIKE @table_pattern
			AND TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') < @cutoff_date
		ORDER BY partition_date ASC
	`

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query,
		sql.Named("table_pattern", tablePattern),
		sql.Named("cutoff_date", cutoffDate)).Scan(&partitions)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to get partitions for removal")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":          tableName,
		"retention_days": retentionDays,
		"cutoff_date":    cutoffDate.Format("2006-01-02"),
		"count":          len(partitions),
		"elapsed":        elapsed,
	}).Info("identified partitions for removal")

	return partitions, nil
}

// GetRetentionSummary provides a summary of what would be affected by a retention policy for a given table
func GetRetentionSummary(dbc *db.DB, tableName string, retentionDays int) (*RetentionSummary, error) {
	start := time.Now()

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	var summary RetentionSummary
	summary.RetentionDays = retentionDays
	summary.CutoffDate = cutoffDate

	query := `
		SELECT
			COUNT(*)::INT AS partitions_to_remove,
			COALESCE(SUM(pg_total_relation_size('public.'||tablename)), 0)::BIGINT AS storage_to_reclaim,
			COALESCE(pg_size_pretty(SUM(pg_total_relation_size('public.'||tablename))), '0 bytes') AS storage_pretty,
			MIN(tablename) AS oldest_partition,
			MAX(tablename) AS newest_partition
		FROM pg_tables
		WHERE schemaname = 'public'
			AND tablename LIKE @table_pattern
			AND TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') < @cutoff_date
	`

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query,
		sql.Named("table_pattern", tablePattern),
		sql.Named("cutoff_date", cutoffDate)).Scan(&summary)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to get retention summary")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":                tableName,
		"retention_days":       retentionDays,
		"partitions_to_remove": summary.PartitionsToRemove,
		"storage_to_reclaim":   summary.StoragePretty,
		"elapsed":              elapsed,
	}).Info("calculated retention summary")

	return &summary, nil
}

// GetPartitionsByAgeGroup returns partition counts and sizes grouped by age buckets for a given table
func GetPartitionsByAgeGroup(dbc *db.DB, tableName string) ([]map[string]interface{}, error) {
	start := time.Now()

	query := `
		WITH partition_ages AS (
			SELECT
				tablename,
				TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
				(CURRENT_DATE - TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD'))::INT AS age_days,
				pg_total_relation_size('public.'||tablename) AS size_bytes
			FROM pg_tables
			WHERE schemaname = 'public'
				AND tablename LIKE @table_pattern
		)
		SELECT
			CASE
				WHEN age_days < 0 THEN 'Future'
				WHEN age_days < 30 THEN '0-30 days'
				WHEN age_days < 90 THEN '30-90 days'
				WHEN age_days < 180 THEN '90-180 days'
				WHEN age_days < 365 THEN '180-365 days'
				ELSE '365+ days'
			END AS age_bucket,
			COUNT(*)::INT AS partition_count,
			SUM(size_bytes)::BIGINT AS total_size_bytes,
			pg_size_pretty(SUM(size_bytes)) AS total_size,
			ROUND(SUM(size_bytes) * 100.0 / SUM(SUM(size_bytes)) OVER (), 2) AS percentage
		FROM partition_ages
		GROUP BY age_bucket
		ORDER BY MIN(age_days)
	`

	tablePattern := tableName + "_20%"
	var results []map[string]interface{}
	err := dbc.DB.Raw(query, sql.Named("table_pattern", tablePattern)).Scan(&results).Error
	if err != nil {
		log.WithError(err).WithField("table", tableName).Error("failed to get partitions by age group")
		return nil, err
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":   tableName,
		"groups":  len(results),
		"elapsed": elapsed,
	}).Info("retrieved partitions by age group")

	return results, nil
}

// GetPartitionsByMonth returns partition counts and sizes grouped by month for a given table
func GetPartitionsByMonth(dbc *db.DB, tableName string) ([]map[string]interface{}, error) {
	start := time.Now()

	query := `
		SELECT
			DATE_TRUNC('month', TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD')) AS month,
			COUNT(*)::INT AS partition_count,
			pg_size_pretty(SUM(pg_total_relation_size('public.'||tablename))) AS total_size,
			pg_size_pretty(AVG(pg_total_relation_size('public.'||tablename))::BIGINT) AS avg_partition_size,
			MIN(tablename) AS first_partition,
			MAX(tablename) AS last_partition
		FROM pg_tables
		WHERE schemaname = 'public'
			AND tablename LIKE @table_pattern
		GROUP BY DATE_TRUNC('month', TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD'))
		ORDER BY month DESC
	`

	tablePattern := tableName + "_20%"
	var results []map[string]interface{}
	err := dbc.DB.Raw(query, sql.Named("table_pattern", tablePattern)).Scan(&results).Error
	if err != nil {
		log.WithError(err).WithField("table", tableName).Error("failed to get partitions by month")
		return nil, err
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":   tableName,
		"months":  len(results),
		"elapsed": elapsed,
	}).Info("retrieved partitions by month")

	return results, nil
}

// ValidateRetentionPolicy checks if a retention policy would be safe to apply for a given table
// Returns an error if the policy would delete critical data or too much data
// Only considers attached partitions when validating thresholds
func ValidateRetentionPolicy(dbc *db.DB, tableName string, retentionDays int) error {
	// Minimum retention is 90 days
	if retentionDays < 90 {
		return fmt.Errorf("retention policy too aggressive: minimum 90 days required, got %d", retentionDays)
	}

	summary, err := GetRetentionSummary(dbc, tableName, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to get retention summary: %w", err)
	}

	// Get stats for attached partitions only (detached partitions are not considered)
	stats, err := GetAttachedPartitionStats(dbc, tableName)
	if err != nil {
		return fmt.Errorf("failed to get attached partition stats: %w", err)
	}

	// Check if we'd delete more than 75% of attached partitions
	if stats.TotalPartitions > 0 {
		deletePercentage := float64(summary.PartitionsToRemove) / float64(stats.TotalPartitions) * 100
		if deletePercentage > 75 {
			return fmt.Errorf("retention policy would delete %.1f%% of attached partitions (%d of %d) - exceeds 75%% safety threshold",
				deletePercentage, summary.PartitionsToRemove, stats.TotalPartitions)
		}
	}

	// Check if we'd delete more than 80% of storage from attached partitions
	if stats.TotalSizeBytes > 0 {
		deletePercentage := float64(summary.StorageToReclaim) / float64(stats.TotalSizeBytes) * 100
		if deletePercentage > 80 {
			return fmt.Errorf("retention policy would delete %.1f%% of attached storage (%s of %s) - exceeds 80%% safety threshold",
				deletePercentage, summary.StoragePretty, stats.TotalSizePretty)
		}
	}

	log.WithFields(log.Fields{
		"table":                tableName,
		"retention_days":       retentionDays,
		"partitions_to_remove": summary.PartitionsToRemove,
		"attached_partitions":  stats.TotalPartitions,
		"attached_storage":     stats.TotalSizePretty,
		"storage_to_reclaim":   summary.StoragePretty,
	}).Info("retention policy validated")

	return nil
}

// DropPartition drops a single partition (DESTRUCTIVE - requires write access)
// This is a wrapper around DROP TABLE for safety and logging
func DropPartition(dbc *db.DB, partitionName string, dryRun bool) error {
	start := time.Now()

	// Extract table name from partition name
	tableName, err := extractTableNameFromPartition(partitionName)
	if err != nil {
		return fmt.Errorf("invalid partition name: %w", err)
	}

	// Validate partition name format for safety
	if !isValidPartitionName(tableName, partitionName) {
		return fmt.Errorf("invalid partition name: %s - must match %s_YYYY_MM_DD", partitionName, tableName)
	}

	if dryRun {
		log.WithFields(log.Fields{
			"partition": partitionName,
			"table":     tableName,
		}).Info("[DRY RUN] would drop partition")
		return nil
	}

	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", partitionName)
	result := dbc.DB.Exec(query)
	if result.Error != nil {
		log.WithError(result.Error).WithFields(log.Fields{
			"partition": partitionName,
			"table":     tableName,
		}).Error("failed to drop partition")
		return result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"partition": partitionName,
		"table":     tableName,
		"elapsed":   elapsed,
	}).Info("dropped partition")

	return nil
}

// DetachPartition detaches a partition from the parent table (safer alternative to DROP)
// The detached table can be archived or dropped later
func DetachPartition(dbc *db.DB, partitionName string, dryRun bool) error {
	start := time.Now()

	// Extract table name from partition name
	tableName, err := extractTableNameFromPartition(partitionName)
	if err != nil {
		return fmt.Errorf("invalid partition name: %w", err)
	}

	// Validate partition name format for safety
	if !isValidPartitionName(tableName, partitionName) {
		return fmt.Errorf("invalid partition name: %s - must match %s_YYYY_MM_DD", partitionName, tableName)
	}

	if dryRun {
		log.WithFields(log.Fields{
			"partition": partitionName,
			"table":     tableName,
		}).Info("[DRY RUN] would detach partition")
		return nil
	}

	query := fmt.Sprintf("ALTER TABLE %s DETACH PARTITION %s", tableName, partitionName)
	result := dbc.DB.Exec(query)
	if result.Error != nil {
		log.WithError(result.Error).WithFields(log.Fields{
			"partition": partitionName,
			"table":     tableName,
		}).Error("failed to detach partition")
		return result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"partition": partitionName,
		"table":     tableName,
		"elapsed":   elapsed,
	}).Info("detached partition")

	return nil
}

// DropOldPartitions drops all partitions older than the retention period for a given table
// This is a bulk operation wrapper that calls DropPartition for each old partition
func DropOldPartitions(dbc *db.DB, tableName string, retentionDays int, dryRun bool) (int, error) {
	start := time.Now()

	// Validate retention policy first
	if err := ValidateRetentionPolicy(dbc, tableName, retentionDays); err != nil {
		return 0, fmt.Errorf("retention policy validation failed: %w", err)
	}

	partitions, err := GetPartitionsForRemoval(dbc, tableName, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to get partitions for removal: %w", err)
	}

	if len(partitions) == 0 {
		log.WithField("table", tableName).Info("no partitions to delete")
		return 0, nil
	}

	droppedCount := 0
	var totalSize int64

	for _, partition := range partitions {
		if err := DropPartition(dbc, partition.TableName, dryRun); err != nil {
			log.WithError(err).WithField("partition", partition.TableName).Error("failed to drop partition")
			continue
		}
		droppedCount++
		totalSize += partition.SizeBytes
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":             tableName,
		"retention_days":    retentionDays,
		"total_dropped":     droppedCount,
		"storage_reclaimed": fmt.Sprintf("%d bytes", totalSize),
		"dry_run":           dryRun,
		"elapsed":           elapsed,
	}).Info("completed dropping old partitions")

	return droppedCount, nil
}

// DropOldDetachedPartitions drops detached partitions older than retentionDays (DESTRUCTIVE)
// This removes detached partitions that are no longer needed
// Use this after archiving detached partitions or when you're sure the data is no longer needed
func DropOldDetachedPartitions(dbc *db.DB, tableName string, retentionDays int, dryRun bool) (int, error) {
	start := time.Now()

	// Get all detached partitions
	detached, err := ListDetachedPartitions(dbc, tableName)
	if err != nil {
		return 0, fmt.Errorf("failed to list detached partitions: %w", err)
	}

	if len(detached) == 0 {
		log.WithField("table", tableName).Info("no detached partitions found")
		return 0, nil
	}

	// Filter by retention period
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)
	var toRemove []PartitionInfo

	for _, partition := range detached {
		if partition.PartitionDate.Before(cutoffDate) {
			toRemove = append(toRemove, partition)
		}
	}

	if len(toRemove) == 0 {
		log.WithFields(log.Fields{
			"table":          tableName,
			"retention_days": retentionDays,
			"cutoff_date":    cutoffDate.Format("2006-01-02"),
		}).Info("no detached partitions older than retention period")
		return 0, nil
	}

	// Drop each old detached partition
	droppedCount := 0
	var totalSize int64

	for _, partition := range toRemove {
		if err := DropPartition(dbc, partition.TableName, dryRun); err != nil {
			log.WithError(err).WithField("partition", partition.TableName).Error("failed to drop detached partition")
			continue
		}
		droppedCount++
		totalSize += partition.SizeBytes
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":             tableName,
		"retention_days":    retentionDays,
		"total_dropped":     droppedCount,
		"storage_reclaimed": fmt.Sprintf("%d bytes", totalSize),
		"dry_run":           dryRun,
		"elapsed":           elapsed,
	}).Info("completed dropping old detached partitions")

	return droppedCount, nil
}

// ListDetachedPartitions returns partitions that have been detached from the parent table
// Detached partitions are standalone tables that match the naming pattern but are no longer
// part of the partitioned table hierarchy
func ListDetachedPartitions(dbc *db.DB, tableName string) ([]PartitionInfo, error) {
	start := time.Now()
	var partitions []PartitionInfo

	query := `
		WITH attached_partitions AS (
			-- Get all currently attached partitions using pg_inherits
			SELECT c.relname AS tablename
			FROM pg_inherits i
			JOIN pg_class c ON i.inhrelid = c.oid
			JOIN pg_class p ON i.inhparent = p.oid
			WHERE p.relname = @table_name
		)
		SELECT
			tablename,
			'public' as schemaname,
			TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
			(CURRENT_DATE - TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD'))::INT AS age_days,
			pg_total_relation_size('public.'||tablename) AS size_bytes,
			pg_size_pretty(pg_total_relation_size('public.'||tablename)) AS size_pretty,
			COALESCE(n_live_tup, 0) AS row_estimate
		FROM pg_tables
		LEFT JOIN pg_stat_user_tables ON pg_stat_user_tables.relname = pg_tables.tablename
			AND pg_stat_user_tables.schemaname = pg_tables.schemaname
		WHERE pg_tables.schemaname = 'public'
			AND pg_tables.tablename LIKE @table_pattern
			AND pg_tables.tablename NOT IN (SELECT tablename FROM attached_partitions)
		ORDER BY partition_date ASC
	`

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query,
		sql.Named("table_name", tableName),
		sql.Named("table_pattern", tablePattern)).Scan(&partitions)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to list detached partitions")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":   tableName,
		"count":   len(partitions),
		"elapsed": elapsed,
	}).Info("listed detached partitions")

	return partitions, nil
}

// ListAttachedPartitions returns partitions that are currently attached to the parent table
// These are partitions that are part of the active partitioned table hierarchy
func ListAttachedPartitions(dbc *db.DB, tableName string) ([]PartitionInfo, error) {
	start := time.Now()
	var partitions []PartitionInfo

	query := `
		WITH attached_partitions AS (
			-- Get all currently attached partitions using pg_inherits
			SELECT c.relname AS tablename
			FROM pg_inherits i
			JOIN pg_class c ON i.inhrelid = c.oid
			JOIN pg_class p ON i.inhparent = p.oid
			WHERE p.relname = @table_name
		)
		SELECT
			tablename,
			'public' as schemaname,
			TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
			(CURRENT_DATE - TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD'))::INT AS age_days,
			pg_total_relation_size('public.'||tablename) AS size_bytes,
			pg_size_pretty(pg_total_relation_size('public.'||tablename)) AS size_pretty,
			COALESCE(n_live_tup, 0) AS row_estimate
		FROM pg_tables
		LEFT JOIN pg_stat_user_tables ON pg_stat_user_tables.relname = pg_tables.tablename
			AND pg_stat_user_tables.schemaname = pg_tables.schemaname
		WHERE pg_tables.schemaname = 'public'
			AND pg_tables.tablename IN (SELECT tablename FROM attached_partitions)
		ORDER BY partition_date ASC
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&partitions)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to list attached partitions")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":   tableName,
		"count":   len(partitions),
		"elapsed": elapsed,
	}).Info("listed attached partitions")

	return partitions, nil
}

// GetAttachedPartitionStats returns statistics about attached partitions for a given table
func GetAttachedPartitionStats(dbc *db.DB, tableName string) (*PartitionStats, error) {
	start := time.Now()
	var stats PartitionStats

	query := `
		WITH attached_partitions AS (
			SELECT c.relname AS tablename
			FROM pg_inherits i
			JOIN pg_class c ON i.inhrelid = c.oid
			JOIN pg_class p ON i.inhparent = p.oid
			WHERE p.relname = @table_name
		),
		attached_info AS (
			SELECT
				tablename,
				TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
				pg_total_relation_size('public.'||tablename) AS size_bytes
			FROM pg_tables
			WHERE schemaname = 'public'
				AND tablename IN (SELECT tablename FROM attached_partitions)
		)
		SELECT
			COALESCE(COUNT(*), 0)::INT AS total_partitions,
			COALESCE(SUM(size_bytes), 0)::BIGINT AS total_size_bytes,
			pg_size_pretty(COALESCE(SUM(size_bytes), 0)) AS total_size_pretty,
			MIN(partition_date) AS oldest_date,
			MAX(partition_date) AS newest_date,
			COALESCE(AVG(size_bytes), 0)::BIGINT AS avg_size_bytes,
			pg_size_pretty(COALESCE(AVG(size_bytes), 0)::BIGINT) AS avg_size_pretty
		FROM attached_info
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&stats)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to get attached partition statistics")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":            tableName,
		"total_partitions": stats.TotalPartitions,
		"total_size":       stats.TotalSizePretty,
		"elapsed":          elapsed,
	}).Info("retrieved attached partition statistics")

	return &stats, nil
}

// GetDetachedPartitionStats returns statistics about detached partitions for a given table
func GetDetachedPartitionStats(dbc *db.DB, tableName string) (*PartitionStats, error) {
	start := time.Now()
	var stats PartitionStats

	query := `
		WITH attached_partitions AS (
			SELECT c.relname AS tablename
			FROM pg_inherits i
			JOIN pg_class c ON i.inhrelid = c.oid
			JOIN pg_class p ON i.inhparent = p.oid
			WHERE p.relname = @table_name
		),
		detached_info AS (
			SELECT
				tablename,
				TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date,
				pg_total_relation_size('public.'||tablename) AS size_bytes
			FROM pg_tables
			WHERE schemaname = 'public'
				AND tablename LIKE @table_pattern
				AND tablename NOT IN (SELECT tablename FROM attached_partitions)
		)
		SELECT
			COUNT(*)::INT AS total_partitions,
			COALESCE(SUM(size_bytes), 0)::BIGINT AS total_size_bytes,
			COALESCE(pg_size_pretty(SUM(size_bytes)), '0 bytes') AS total_size_pretty,
			MIN(partition_date) AS oldest_date,
			MAX(partition_date) AS newest_date,
			COALESCE(AVG(size_bytes), 0)::BIGINT AS avg_size_bytes,
			COALESCE(pg_size_pretty(AVG(size_bytes)::BIGINT), '0 bytes') AS avg_size_pretty
		FROM detached_info
	`

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query,
		sql.Named("table_name", tableName),
		sql.Named("table_pattern", tablePattern)).Scan(&stats)
	if result.Error != nil {
		log.WithError(result.Error).WithField("table", tableName).Error("failed to get detached partition statistics")
		return nil, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":            tableName,
		"total_partitions": stats.TotalPartitions,
		"total_size":       stats.TotalSizePretty,
		"elapsed":          elapsed,
	}).Info("retrieved detached partition statistics")

	return &stats, nil
}

// ReattachPartition reattaches a previously detached partition back to the parent table
// This is useful if a partition was detached for archival but needs to be restored
func ReattachPartition(dbc *db.DB, partitionName string, dryRun bool) error {
	start := time.Now()

	// Extract table name from partition name
	tableName, err := extractTableNameFromPartition(partitionName)
	if err != nil {
		return fmt.Errorf("invalid partition name: %w", err)
	}

	// Validate partition name format for safety
	if !isValidPartitionName(tableName, partitionName) {
		return fmt.Errorf("invalid partition name: %s - must match %s_YYYY_MM_DD", partitionName, tableName)
	}

	// Extract date from partition name
	prefix := tableName + "_"
	dateStr := partitionName[len(prefix):]
	partitionDate, err := time.Parse("2006_01_02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid partition date format: %w", err)
	}

	// Calculate date range for the partition
	startDate := partitionDate.Format("2006-01-02")
	endDate := partitionDate.AddDate(0, 0, 1).Format("2006-01-02")

	if dryRun {
		log.WithFields(log.Fields{
			"partition":  partitionName,
			"table":      tableName,
			"start_date": startDate,
			"end_date":   endDate,
		}).Info("[DRY RUN] would reattach partition")
		return nil
	}

	// Reattach the partition with FOR VALUES clause
	query := fmt.Sprintf(
		"ALTER TABLE %s ATTACH PARTITION %s FOR VALUES FROM ('%s') TO ('%s')",
		tableName,
		partitionName,
		startDate,
		endDate,
	)

	result := dbc.DB.Exec(query)
	if result.Error != nil {
		log.WithError(result.Error).WithFields(log.Fields{
			"partition": partitionName,
			"table":     tableName,
		}).Error("failed to reattach partition")
		return result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"partition": partitionName,
		"table":     tableName,
		"elapsed":   elapsed,
	}).Info("reattached partition")

	return nil
}

// IsPartitionAttached checks if a partition is currently attached to the parent table
func IsPartitionAttached(dbc *db.DB, partitionName string) (bool, error) {
	start := time.Now()

	// Extract table name from partition name
	tableName, err := extractTableNameFromPartition(partitionName)
	if err != nil {
		return false, fmt.Errorf("invalid partition name: %w", err)
	}

	// Validate partition name format for safety
	if !isValidPartitionName(tableName, partitionName) {
		return false, fmt.Errorf("invalid partition name: %s", partitionName)
	}

	var isAttached bool
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM pg_inherits i
			JOIN pg_class c ON i.inhrelid = c.oid
			JOIN pg_class p ON i.inhparent = p.oid
			WHERE p.relname = @table_name
				AND c.relname = @partition_name
		) AS is_attached
	`

	result := dbc.DB.Raw(query,
		sql.Named("table_name", tableName),
		sql.Named("partition_name", partitionName)).Scan(&isAttached)
	if result.Error != nil {
		log.WithError(result.Error).WithFields(log.Fields{
			"partition": partitionName,
			"table":     tableName,
		}).Error("failed to check partition status")
		return false, result.Error
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"partition":   partitionName,
		"table":       tableName,
		"is_attached": isAttached,
		"elapsed":     elapsed,
	}).Debug("checked partition attachment status")

	return isAttached, nil
}

// DetachOldPartitions detaches all partitions older than the retention period for a given table
// This is safer than dropping as partitions can be reattached if needed
func DetachOldPartitions(dbc *db.DB, tableName string, retentionDays int, dryRun bool) (int, error) {
	start := time.Now()

	// Validate retention policy first
	if err := ValidateRetentionPolicy(dbc, tableName, retentionDays); err != nil {
		return 0, fmt.Errorf("retention policy validation failed: %w", err)
	}

	partitions, err := GetPartitionsForRemoval(dbc, tableName, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to get partitions for removal: %w", err)
	}

	if len(partitions) == 0 {
		log.WithField("table", tableName).Info("no partitions to detach")
		return 0, nil
	}

	detachedCount := 0
	var totalSize int64

	for _, partition := range partitions {
		if err := DetachPartition(dbc, partition.TableName, dryRun); err != nil {
			log.WithError(err).WithField("partition", partition.TableName).Error("failed to detach partition")
			continue
		}
		detachedCount++
		totalSize += partition.SizeBytes
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":            tableName,
		"retention_days":   retentionDays,
		"total_detached":   detachedCount,
		"storage_affected": fmt.Sprintf("%d bytes", totalSize),
		"dry_run":          dryRun,
		"elapsed":          elapsed,
	}).Info("completed detaching old partitions")

	return detachedCount, nil
}

// extractTableNameFromPartition extracts the table name from a partition name
// Partition format: {tablename}_YYYY_MM_DD
func extractTableNameFromPartition(partitionName string) (string, error) {
	// Must end with _YYYY_MM_DD (10 characters + 1 underscore = 11)
	if len(partitionName) < 12 {
		return "", fmt.Errorf("partition name too short: %s", partitionName)
	}

	// Extract the date portion (last 10 characters should be YYYY_MM_DD)
	dateStr := partitionName[len(partitionName)-10:]

	// Validate date format
	_, err := time.Parse("2006_01_02", dateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date format in partition name: %s", partitionName)
	}

	// Table name is everything except the last 11 characters (_YYYY_MM_DD)
	tableName := partitionName[:len(partitionName)-11]

	return tableName, nil
}

// isValidPartitionName validates that a partition name matches the expected format for a given table
// This is a safety check to prevent SQL injection and accidental drops
func isValidPartitionName(tableName, partitionName string) bool {
	expectedPrefix := tableName + "_"
	expectedLen := len(expectedPrefix) + 10 // prefix + "YYYY_MM_DD"

	if len(partitionName) != expectedLen {
		return false
	}

	if !strings.HasPrefix(partitionName, expectedPrefix) {
		return false
	}

	// Must start with 20xx (year 2000-2099)
	if len(partitionName) < len(expectedPrefix)+2 || partitionName[len(expectedPrefix):len(expectedPrefix)+2] != "20" {
		return false
	}

	// Validate date format by parsing
	dateStr := partitionName[len(expectedPrefix):] // YYYY_MM_DD format
	_, err := time.Parse("2006_01_02", dateStr)
	return err == nil
}
