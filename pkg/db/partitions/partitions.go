package partitions

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

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

// PartitionConfig defines the configuration for creating a partitioned table
type PartitionConfig struct {
	// Strategy is the partitioning strategy (RANGE, LIST, or HASH)
	Strategy db.PartitionStrategy

	// Columns are the column(s) to partition by
	// For RANGE and LIST: typically one column (e.g., "date", "created_at")
	// For HASH: can be one or more columns
	Columns []string

	// Modulus is required for HASH partitioning (number of partitions)
	// Not used for RANGE or LIST
	Modulus int
}

// NewRangePartitionConfig creates a partition config for RANGE partitioning
func NewRangePartitionConfig(column string) PartitionConfig {
	return PartitionConfig{
		Strategy: db.PartitionStrategyRange,
		Columns:  []string{column},
	}
}

// NewListPartitionConfig creates a partition config for LIST partitioning
func NewListPartitionConfig(column string) PartitionConfig {
	return PartitionConfig{
		Strategy: db.PartitionStrategyList,
		Columns:  []string{column},
	}
}

// NewHashPartitionConfig creates a partition config for HASH partitioning
func NewHashPartitionConfig(modulus int, columns ...string) PartitionConfig {
	return PartitionConfig{
		Strategy: db.PartitionStrategyHash,
		Columns:  columns,
		Modulus:  modulus,
	}
}

// Validate checks if the partition configuration is valid
func (pc PartitionConfig) Validate() error {
	if pc.Strategy == "" {
		return fmt.Errorf("partition strategy must be specified")
	}

	if len(pc.Columns) == 0 {
		return fmt.Errorf("at least one partition column must be specified")
	}

	switch pc.Strategy {
	case db.PartitionStrategyRange, db.PartitionStrategyList:
		if len(pc.Columns) != 1 {
			return fmt.Errorf("%s partitioning requires exactly one column, got %d", pc.Strategy, len(pc.Columns))
		}
	case db.PartitionStrategyHash:
		if pc.Modulus <= 0 {
			return fmt.Errorf("HASH partitioning requires modulus > 0, got %d", pc.Modulus)
		}
	default:
		return fmt.Errorf("unknown partition strategy: %s (valid: RANGE, LIST, HASH)", pc.Strategy)
	}

	return nil
}

// ToSQL generates the PARTITION BY clause for the CREATE TABLE statement
func (pc PartitionConfig) ToSQL() string {
	columnList := strings.Join(pc.Columns, ", ")

	switch pc.Strategy {
	case db.PartitionStrategyRange:
		return fmt.Sprintf("PARTITION BY RANGE (%s)", columnList)
	case db.PartitionStrategyList:
		return fmt.Sprintf("PARTITION BY LIST (%s)", columnList)
	case db.PartitionStrategyHash:
		return fmt.Sprintf("PARTITION BY HASH (%s)", columnList)
	default:
		return ""
	}
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
// If attachedOnly is true, only returns attached partitions (useful for detach operations)
// If attachedOnly is false, returns all partitions (useful for drop operations on both attached and detached)
func GetPartitionsForRemoval(dbc *db.DB, tableName string, retentionDays int, attachedOnly bool) ([]PartitionInfo, error) {
	start := time.Now()
	var partitions []PartitionInfo

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	var query string
	if attachedOnly {
		// Only return attached partitions
		query = `
			WITH attached_partitions AS (
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
				AND pg_tables.tablename IN (SELECT tablename FROM attached_partitions)
				AND TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') < @cutoff_date
			ORDER BY partition_date ASC
		`
	} else {
		// Return all partitions (attached + detached)
		query = `
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
	}

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query,
		sql.Named("table_name", tableName),
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
		"attached_only":  attachedOnly,
		"count":          len(partitions),
		"elapsed":        elapsed,
	}).Info("identified partitions for removal")

	return partitions, nil
}

// GetRetentionSummary provides a summary of what would be affected by a retention policy for a given table
// If attachedOnly is true, only considers attached partitions (useful for detach operations)
// If attachedOnly is false, considers all partitions (useful for drop operations on both attached and detached)
func GetRetentionSummary(dbc *db.DB, tableName string, retentionDays int, attachedOnly bool) (*RetentionSummary, error) {
	start := time.Now()

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	var summary RetentionSummary
	summary.RetentionDays = retentionDays
	summary.CutoffDate = cutoffDate

	var query string
	if attachedOnly {
		// Only consider attached partitions
		query = `
			WITH attached_partitions AS (
				SELECT c.relname AS tablename
				FROM pg_inherits i
				JOIN pg_class c ON i.inhrelid = c.oid
				JOIN pg_class p ON i.inhparent = p.oid
				WHERE p.relname = @table_name
			)
			SELECT
				COUNT(*)::INT AS partitions_to_remove,
				COALESCE(SUM(pg_total_relation_size('public.'||tablename)), 0)::BIGINT AS storage_to_reclaim,
				COALESCE(pg_size_pretty(SUM(pg_total_relation_size('public.'||tablename))), '0 bytes') AS storage_pretty,
				MIN(tablename) AS oldest_partition,
				MAX(tablename) AS newest_partition
			FROM pg_tables
			WHERE schemaname = 'public'
				AND tablename LIKE @table_pattern
				AND tablename IN (SELECT tablename FROM attached_partitions)
				AND TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') < @cutoff_date
		`
	} else {
		// Consider all partitions (attached + detached)
		query = `
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
	}

	tablePattern := tableName + "_20%"
	result := dbc.DB.Raw(query,
		sql.Named("table_name", tableName),
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
		"attached_only":        attachedOnly,
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

	// Get summary for attached partitions only to match stats below
	summary, err := GetRetentionSummary(dbc, tableName, retentionDays, true)
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

	// Get all partitions for removal (both attached and detached)
	partitions, err := GetPartitionsForRemoval(dbc, tableName, retentionDays, false)
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

// CreateMissingPartitions creates partitions for a date range if they don't already exist
// Assumes daily partitions (one partition per day) based on the naming convention: tablename_YYYY_MM_DD
// Each partition covers a 24-hour period from midnight to midnight
//
// Workflow:
//  1. Lists all existing partitions (both attached and detached)
//  2. Generates list of missing dates in the specified range
//  3. For each missing date: creates table and attaches it as partition
//  4. Skips dates that already have partitions (attached or detached)
//
// Parameters:
//   - tableName: Name of the partitioned parent table
//   - startDate: Start of date range (inclusive)
//   - endDate: End of date range (inclusive)
//   - dryRun: If true, logs what would be created without executing
//
// Returns: Count of partitions created (or would be created in dry-run mode)
func CreateMissingPartitions(dbc *db.DB, tableName string, startDate, endDate time.Time, dryRun bool) (int, error) {
	start := time.Now()

	// Validate date range
	if endDate.Before(startDate) {
		return 0, fmt.Errorf("end date (%s) cannot be before start date (%s)",
			endDate.Format("2006-01-02"), startDate.Format("2006-01-02"))
	}

	// Get list of all existing partitions (attached + detached)
	existingPartitions, err := ListTablePartitions(dbc, tableName)
	if err != nil {
		return 0, fmt.Errorf("failed to list existing partitions: %w", err)
	}

	// Create a map of existing partition dates for quick lookup
	existingDates := make(map[string]bool)
	for _, p := range existingPartitions {
		dateStr := p.PartitionDate.Format("2006_01_02")
		existingDates[dateStr] = true
	}

	// Generate list of partitions to create
	var partitionsToCreate []time.Time
	currentDate := startDate
	for !currentDate.After(endDate) {
		dateStr := currentDate.Format("2006_01_02")
		if !existingDates[dateStr] {
			partitionsToCreate = append(partitionsToCreate, currentDate)
		}
		currentDate = currentDate.AddDate(0, 0, 1) // Move to next day
	}

	if len(partitionsToCreate) == 0 {
		log.WithFields(log.Fields{
			"table":      tableName,
			"start_date": startDate.Format("2006-01-02"),
			"end_date":   endDate.Format("2006-01-02"),
		}).Info("no missing partitions to create")
		return 0, nil
	}

	createdCount := 0
	for _, partitionDate := range partitionsToCreate {
		partitionName := fmt.Sprintf("%s_%s", tableName, partitionDate.Format("2006_01_02"))
		rangeStart := partitionDate.Format("2006-01-02")
		rangeEnd := partitionDate.AddDate(0, 0, 1).Format("2006-01-02")

		if dryRun {
			log.WithFields(log.Fields{
				"partition":   partitionName,
				"table":       tableName,
				"range_start": rangeStart,
				"range_end":   rangeEnd,
			}).Info("[DRY RUN] would create partition")
			createdCount++
			continue
		}

		// Create the partition table with same structure as parent
		createTableQuery := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (LIKE %s INCLUDING ALL)", partitionName, tableName)
		result := dbc.DB.Exec(createTableQuery)
		if result.Error != nil {
			log.WithError(result.Error).WithField("partition", partitionName).Error("failed to create partition table")
			continue
		}

		// Attach the partition to the parent table
		attachQuery := fmt.Sprintf(
			"ALTER TABLE %s ATTACH PARTITION %s FOR VALUES FROM ('%s') TO ('%s')",
			tableName,
			partitionName,
			rangeStart,
			rangeEnd,
		)
		result = dbc.DB.Exec(attachQuery)
		if result.Error != nil {
			// If attach fails, try to clean up the created table
			log.WithError(result.Error).WithField("partition", partitionName).Error("failed to attach partition")
			dbc.DB.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", partitionName))
			continue
		}

		log.WithFields(log.Fields{
			"partition":   partitionName,
			"table":       tableName,
			"range_start": rangeStart,
			"range_end":   rangeEnd,
		}).Info("created and attached partition")
		createdCount++
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":      tableName,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
		"created":    createdCount,
		"total_days": len(partitionsToCreate),
		"dry_run":    dryRun,
		"elapsed":    elapsed,
	}).Info("completed creating missing partitions")

	return createdCount, nil
}

// gormTypeToPostgresType converts GORM/Go data types to PostgreSQL types
func gormTypeToPostgresType(dataType string) string {
	dataType = strings.ToLower(strings.TrimSpace(dataType))

	// Map GORM/Go types to PostgreSQL types
	typeMap := map[string]string{
		// Integer types
		"uint":    "bigint",
		"uint8":   "smallint",
		"uint16":  "integer",
		"uint32":  "bigint",
		"uint64":  "bigint",
		"int":     "bigint",
		"int8":    "smallint",
		"int16":   "smallint",
		"int32":   "integer",
		"int64":   "bigint",
		"integer": "integer",
		"bigint":  "bigint",

		// Float types
		"float":   "double precision",
		"float32": "real",
		"float64": "double precision",

		// String types
		"string": "text",
		"text":   "text",

		// Boolean
		"bool":    "boolean",
		"boolean": "boolean",

		// Time types
		"time.time": "timestamp with time zone",
		"time":      "timestamp with time zone",
		"timestamp": "timestamp with time zone",
		"date":      "date",

		// Binary
		"[]byte": "bytea",
		"bytes":  "bytea",
		"bytea":  "bytea",

		// JSON
		"json":  "jsonb",
		"jsonb": "jsonb",

		// UUID
		"uuid": "uuid",
	}

	// Check if we have a direct mapping
	if pgType, exists := typeMap[dataType]; exists {
		return pgType
	}

	// If it's already a PostgreSQL type, return as-is
	// Common PostgreSQL types that might pass through
	postgresTypes := []string{
		"varchar", "character varying",
		"smallint", "bigserial", "serial",
		"numeric", "decimal", "real", "double precision",
		"timestamptz", "timestamp without time zone",
		"interval", "money",
		"inet", "cidr", "macaddr",
		"point", "line", "lseg", "box", "path", "polygon", "circle",
		"xml", "array",
	}

	for _, pgType := range postgresTypes {
		if strings.Contains(dataType, pgType) {
			return dataType
		}
	}

	// If we can't map it, log a warning and return as-is
	// This allows for custom types or types we haven't mapped yet
	log.WithField("data_type", dataType).Warn("unmapped data type - using as-is (may cause PostgreSQL error)")
	return dataType
}

// CreatePartitionedTable creates a new partitioned table based on a GORM model struct
// If the table already exists, it returns without error
//
// Parameters:
//   - model: GORM model struct (must be a pointer, e.g., &models.MyModel{})
//   - tableName: Name for the partitioned table
//   - config: Partition configuration (strategy, columns, etc.)
//   - dryRun: If true, prints SQL without executing
//
// Returns: The SQL statement that was (or would be) executed
//
// Example:
//
//	config := partitions.NewRangePartitionConfig("created_at")
//	sql, err := partitions.CreatePartitionedTable(dbc, &MyModel{}, "my_table", config, true)
func CreatePartitionedTable(dbc *db.DB, model interface{}, tableName string, config PartitionConfig, dryRun bool) (string, error) {
	start := time.Now()

	// Validate partition configuration
	if err := config.Validate(); err != nil {
		return "", fmt.Errorf("invalid partition config: %w", err)
	}

	// Check if table already exists
	if dbc.DB.Migrator().HasTable(tableName) {
		log.WithField("table", tableName).Info("partitioned table already exists, skipping creation")
		return "", nil
	}

	// Use GORM statement parser to get the table structure from the model
	stmt := &gorm.Statement{DB: dbc.DB}
	if err := stmt.Parse(model); err != nil {
		return "", fmt.Errorf("failed to parse model: %w", err)
	}

	// Build the CREATE TABLE statement manually from the GORM schema
	var columns []string
	var primaryKeyColumns []string

	// Create a map of fields with default database values for quick lookup
	hasDefaultDBValue := make(map[string]bool)
	for _, field := range stmt.Schema.FieldsWithDefaultDBValue {
		hasDefaultDBValue[field.Name] = true
	}

	// Track which columns we've already added to prevent duplicates
	addedColumns := make(map[string]bool)

	for _, field := range stmt.Schema.Fields {
		// Skip fields that shouldn't be in the database
		if field.IgnoreMigration {
			continue
		}

		// Skip fields with empty DBName or DataType
		if field.DBName == "" || field.DataType == "" {
			log.WithFields(log.Fields{
				"table":     tableName,
				"field":     field.Name,
				"db_name":   field.DBName,
				"data_type": field.DataType,
			}).Warn("skipping field with empty DBName or DataType")
			continue
		}

		// Skip duplicate columns (GORM can include same field multiple times)
		if addedColumns[field.DBName] {
			log.WithFields(log.Fields{
				"table":  tableName,
				"column": field.DBName,
				"field":  field.Name,
			}).Debug("skipping duplicate column")
			continue
		}
		addedColumns[field.DBName] = true

		// Convert GORM/Go type to PostgreSQL type
		pgType := gormTypeToPostgresType(string(field.DataType))
		columnDef := fmt.Sprintf("%s %s", field.DBName, pgType)

		// Handle AUTO_INCREMENT using GENERATED BY DEFAULT AS IDENTITY
		// This must be done before NOT NULL and DEFAULT clauses
		if field.AutoIncrement {
			// IDENTITY columns are always NOT NULL, so we add GENERATED BY DEFAULT AS IDENTITY
			if field.AutoIncrementIncrement > 0 {
				columnDef += fmt.Sprintf(" GENERATED BY DEFAULT AS IDENTITY (INCREMENT BY %d)", field.AutoIncrementIncrement)
			} else {
				columnDef += " GENERATED BY DEFAULT AS IDENTITY"
			}
			// IDENTITY columns are automatically NOT NULL, no need to add it explicitly
		} else {
			// Add NOT NULL constraint if applicable
			// Primary keys are always NOT NULL in PostgreSQL
			if field.PrimaryKey || field.NotNull {
				columnDef += " NOT NULL"
			}

			// Add DEFAULT if specified
			// Check both field.DefaultValue and if field is in FieldsWithDefaultDBValue
			if field.DefaultValue != "" {
				columnDef += fmt.Sprintf(" DEFAULT %s", field.DefaultValue)
			} else if hasDefaultDBValue[field.Name] && field.DefaultValueInterface != nil {
				// Field has a database-level default value
				columnDef += fmt.Sprintf(" DEFAULT %v", field.DefaultValueInterface)
			}
		}

		columns = append(columns, columnDef)

		// Track primary key columns
		if field.PrimaryKey {
			primaryKeyColumns = append(primaryKeyColumns, field.DBName)
		}
	}

	// Add PRIMARY KEY constraint if we have primary keys
	// For partitioned tables, the primary key must include all partition columns
	if len(primaryKeyColumns) > 0 {
		// Check if primary key includes all partition columns
		pkMap := make(map[string]bool)
		for _, pk := range primaryKeyColumns {
			pkMap[pk] = true
		}

		// Add missing partition columns to primary key
		missingPartCols := []string{}
		for _, partCol := range config.Columns {
			if !pkMap[partCol] {
				missingPartCols = append(missingPartCols, partCol)
			}
		}

		if len(missingPartCols) > 0 {
			log.WithFields(log.Fields{
				"table":             tableName,
				"primary_keys":      primaryKeyColumns,
				"partition_columns": config.Columns,
				"missing_in_pk":     missingPartCols,
			}).Warn("primary key must include all partition columns - adding partition columns to primary key")
			primaryKeyColumns = append(primaryKeyColumns, missingPartCols...)
		}

		primaryKeyConstraint := fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeyColumns, ", "))
		columns = append(columns, primaryKeyConstraint)
	}

	// Build the CREATE TABLE statement with partition strategy
	partitionClause := config.ToSQL()
	createTableSQL := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (\n    %s\n) %s",
		tableName,
		strings.Join(columns, ",\n    "),
		partitionClause,
	)

	// Create a map of partition columns for easy lookup
	partitionColMap := make(map[string]bool)
	for _, col := range config.Columns {
		partitionColMap[col] = true
	}

	// Add indexes if they exist in the schema
	var indexSQL strings.Builder
	for _, idx := range stmt.Schema.ParseIndexes() {
		// Skip unique indexes that don't include ALL partition keys
		// (they're not allowed in partitioned tables)
		if idx.Class == "UNIQUE" {
			hasAllPartitionKeys := true
			for _, partCol := range config.Columns {
				found := false
				for _, field := range idx.Fields {
					if field.DBName == partCol {
						found = true
						break
					}
				}
				if !found {
					hasAllPartitionKeys = false
					break
				}
			}
			if !hasAllPartitionKeys {
				log.WithFields(log.Fields{
					"table":          tableName,
					"index":          idx.Name,
					"partition_keys": config.Columns,
				}).Warn("skipping unique index without all partition keys (not allowed on partitioned tables)")
				continue
			}
		}

		indexSQL.WriteString("\n")
		if idx.Class == "UNIQUE" {
			indexSQL.WriteString(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (", idx.Name, tableName))
		} else {
			indexSQL.WriteString(fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (", idx.Name, tableName))
		}

		var fieldNames []string
		for _, field := range idx.Fields {
			fieldNames = append(fieldNames, field.DBName)
		}
		indexSQL.WriteString(strings.Join(fieldNames, ", "))
		indexSQL.WriteString(");")
	}

	fullSQL := createTableSQL + ";" + indexSQL.String()

	if dryRun {
		log.WithField("table", tableName).Info("[DRY RUN] would execute SQL:")
		fmt.Println("\n" + strings.Repeat("-", 80))
		fmt.Println(fullSQL)
		fmt.Println(strings.Repeat("-", 80) + "\n")
		return fullSQL, nil
	}

	// Execute the CREATE TABLE statement
	result := dbc.DB.Exec(createTableSQL)
	if result.Error != nil {
		return "", fmt.Errorf("failed to create partitioned table: %w", result.Error)
	}

	// Execute index creation statements
	if indexSQL.Len() > 0 {
		result = dbc.DB.Exec(indexSQL.String())
		if result.Error != nil {
			log.WithError(result.Error).Warn("some indexes may have failed to create")
		}
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":              tableName,
		"partition_strategy": string(config.Strategy),
		"partition_columns":  strings.Join(config.Columns, ", "),
		"elapsed":            elapsed,
	}).Info("created partitioned table")

	return fullSQL, nil
}

// indexInfo holds information about a database index
type indexInfo struct {
	IndexName string
	IsUnique  bool
	Columns   []string
}

// UpdatePartitionedTable updates an existing partitioned table schema based on a GORM model
// Detects differences between the model and current database schema and generates ALTER statements
//
// Parameters:
//   - model: GORM model struct (must be a pointer, e.g., &models.MyModel{})
//   - tableName: Name of the existing partitioned table
//   - dryRun: If true, prints SQL without executing
//
// Returns: The SQL statements that were (or would be) executed
//
// Example:
//
//	sql, err := partitions.UpdatePartitionedTable(dbc, &MyModel{}, "my_table", true)
//
// Note: Cannot modify partition keys or add unique constraints without partition keys
func UpdatePartitionedTable(dbc *db.DB, model interface{}, tableName string, dryRun bool) (string, error) {
	start := time.Now()

	// Check if table exists
	if !dbc.DB.Migrator().HasTable(tableName) {
		return "", fmt.Errorf("table %s does not exist", tableName)
	}

	// Parse the GORM model to get desired schema
	stmt := &gorm.Statement{DB: dbc.DB}
	if err := stmt.Parse(model); err != nil {
		return "", fmt.Errorf("failed to parse model: %w", err)
	}

	// Get current schema from database
	currentColumns, err := dbc.GetTableColumns(tableName)
	if err != nil {
		return "", fmt.Errorf("failed to get current columns: %w", err)
	}

	currentIndexes, err := getCurrentIndexes(dbc, tableName)
	if err != nil {
		return "", fmt.Errorf("failed to get current indexes: %w", err)
	}

	// Get partition columns to validate unique indexes
	partitionColumns, err := getPartitionColumns(dbc, tableName)
	if err != nil {
		return "", fmt.Errorf("failed to get partition columns: %w", err)
	}

	// Build maps for comparison
	currentColMap := make(map[string]db.ColumnInfo)
	for _, col := range currentColumns {
		currentColMap[col.ColumnName] = col
	}

	currentIdxMap := make(map[string]indexInfo)
	for _, idx := range currentIndexes {
		currentIdxMap[idx.IndexName] = idx
	}

	// Create a map of fields with default database values for quick lookup
	hasDefaultDBValue := make(map[string]bool)
	for _, field := range stmt.Schema.FieldsWithDefaultDBValue {
		hasDefaultDBValue[field.Name] = true
	}

	// Track which columns we've already processed to prevent duplicates
	processedColumns := make(map[string]bool)

	// Generate ALTER statements
	var alterStatements []string

	// Check for new or modified columns
	for _, field := range stmt.Schema.Fields {
		if field.IgnoreMigration {
			continue
		}

		// Skip fields with empty DBName or DataType
		if field.DBName == "" || field.DataType == "" {
			log.WithFields(log.Fields{
				"table":     tableName,
				"field":     field.Name,
				"db_name":   field.DBName,
				"data_type": field.DataType,
			}).Warn("skipping field with empty DBName or DataType")
			continue
		}

		// Skip duplicate columns (GORM can include same field multiple times)
		if processedColumns[field.DBName] {
			log.WithFields(log.Fields{
				"table":  tableName,
				"column": field.DBName,
				"field":  field.Name,
			}).Debug("skipping duplicate column")
			continue
		}
		processedColumns[field.DBName] = true

		currentCol, exists := currentColMap[field.DBName]

		// Convert GORM/Go type to PostgreSQL type
		pgType := gormTypeToPostgresType(string(field.DataType))

		if !exists {
			// New column - add it
			columnDef := fmt.Sprintf("%s %s", field.DBName, pgType)

			// Handle AUTO_INCREMENT using GENERATED BY DEFAULT AS IDENTITY
			if field.AutoIncrement {
				// IDENTITY columns are always NOT NULL, so we add GENERATED BY DEFAULT AS IDENTITY
				if field.AutoIncrementIncrement > 0 {
					columnDef += fmt.Sprintf(" GENERATED BY DEFAULT AS IDENTITY (INCREMENT BY %d)", field.AutoIncrementIncrement)
				} else {
					columnDef += " GENERATED BY DEFAULT AS IDENTITY"
				}
				// IDENTITY columns are automatically NOT NULL, no need to add it explicitly
			} else {
				// Primary keys are always NOT NULL in PostgreSQL
				if field.PrimaryKey || field.NotNull {
					columnDef += " NOT NULL"
				}
				// Add DEFAULT if specified
				// Check both field.DefaultValue and if field is in FieldsWithDefaultDBValue
				if field.DefaultValue != "" {
					columnDef += fmt.Sprintf(" DEFAULT %s", field.DefaultValue)
				} else if hasDefaultDBValue[field.Name] && field.DefaultValueInterface != nil {
					// Field has a database-level default value
					columnDef += fmt.Sprintf(" DEFAULT %v", field.DefaultValueInterface)
				}
			}

			alterStatements = append(alterStatements,
				fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", tableName, columnDef))
		} else {
			// Existing column - check for modifications
			modifications := []string{}

			// Check data type
			if !strings.EqualFold(normalizeDataType(currentCol.DataType), normalizeDataType(pgType)) {
				modifications = append(modifications,
					fmt.Sprintf("ALTER COLUMN %s TYPE %s", field.DBName, pgType))
			}

			// Check NOT NULL constraint
			// Primary keys are always NOT NULL in PostgreSQL
			currentNotNull := currentCol.IsNullable == "NO"
			desiredNotNull := field.PrimaryKey || field.NotNull
			if desiredNotNull != currentNotNull {
				if desiredNotNull {
					modifications = append(modifications,
						fmt.Sprintf("ALTER COLUMN %s SET NOT NULL", field.DBName))
				} else {
					modifications = append(modifications,
						fmt.Sprintf("ALTER COLUMN %s DROP NOT NULL", field.DBName))
				}
			}

			// Check DEFAULT value
			currentDefault := ""
			if currentCol.ColumnDefault.Valid {
				currentDefault = currentCol.ColumnDefault.String
			}
			if field.DefaultValue != currentDefault {
				if field.DefaultValue != "" {
					modifications = append(modifications,
						fmt.Sprintf("ALTER COLUMN %s SET DEFAULT %s", field.DBName, field.DefaultValue))
				} else if currentDefault != "" {
					modifications = append(modifications,
						fmt.Sprintf("ALTER COLUMN %s DROP DEFAULT", field.DBName))
				}
			}

			// Add modifications as separate ALTER TABLE statements
			for _, mod := range modifications {
				alterStatements = append(alterStatements,
					fmt.Sprintf("ALTER TABLE %s %s", tableName, mod))
			}
		}

		// Remove from map to track processed columns
		delete(currentColMap, field.DBName)
	}

	// Remaining columns in map should be dropped
	for colName := range currentColMap {
		alterStatements = append(alterStatements,
			fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", tableName, colName))
	}

	// Check indexes
	partitionColMap := make(map[string]bool)
	for _, col := range partitionColumns {
		partitionColMap[col] = true
	}

	for _, idx := range stmt.Schema.ParseIndexes() {
		// Skip unique indexes that don't include all partition keys
		if idx.Class == "UNIQUE" {
			hasAllPartitionKeys := true
			for _, partCol := range partitionColumns {
				found := false
				for _, field := range idx.Fields {
					if field.DBName == partCol {
						found = true
						break
					}
				}
				if !found {
					hasAllPartitionKeys = false
					break
				}
			}
			if !hasAllPartitionKeys {
				log.WithFields(log.Fields{
					"table":          tableName,
					"index":          idx.Name,
					"partition_keys": partitionColumns,
				}).Warn("skipping unique index without all partition keys")
				continue
			}
		}

		currentIdx, exists := currentIdxMap[idx.Name]
		if !exists {
			// New index - create it
			var fieldNames []string
			for _, field := range idx.Fields {
				fieldNames = append(fieldNames, field.DBName)
			}

			if idx.Class == "UNIQUE" {
				alterStatements = append(alterStatements,
					fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)",
						idx.Name, tableName, strings.Join(fieldNames, ", ")))
			} else {
				alterStatements = append(alterStatements,
					fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
						idx.Name, tableName, strings.Join(fieldNames, ", ")))
			}
		} else {
			// Index exists - check if it needs to be recreated
			var desiredCols []string
			for _, field := range idx.Fields {
				desiredCols = append(desiredCols, field.DBName)
			}

			colsMatch := len(currentIdx.Columns) == len(desiredCols)
			if colsMatch {
				for i, col := range currentIdx.Columns {
					if col != desiredCols[i] {
						colsMatch = false
						break
					}
				}
			}

			uniqueMatch := (idx.Class == "UNIQUE") == currentIdx.IsUnique

			if !colsMatch || !uniqueMatch {
				// Drop and recreate index
				alterStatements = append(alterStatements,
					fmt.Sprintf("DROP INDEX IF EXISTS %s", idx.Name))

				if idx.Class == "UNIQUE" {
					alterStatements = append(alterStatements,
						fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s)",
							idx.Name, tableName, strings.Join(desiredCols, ", ")))
				} else {
					alterStatements = append(alterStatements,
						fmt.Sprintf("CREATE INDEX %s ON %s (%s)",
							idx.Name, tableName, strings.Join(desiredCols, ", ")))
				}
			}
		}

		delete(currentIdxMap, idx.Name)
	}

	// Drop indexes that are no longer in the model
	for idxName := range currentIdxMap {
		// Skip primary key and system indexes
		if strings.HasSuffix(idxName, "_pkey") {
			continue
		}
		alterStatements = append(alterStatements,
			fmt.Sprintf("DROP INDEX IF EXISTS %s", idxName))
	}

	// If no changes, return early
	if len(alterStatements) == 0 {
		log.WithField("table", tableName).Info("schema is up to date, no changes needed")
		return "", nil
	}

	fullSQL := strings.Join(alterStatements, ";\n") + ";"

	if dryRun {
		log.WithField("table", tableName).Info("[DRY RUN] would execute SQL:")
		fmt.Println("\n" + strings.Repeat("-", 80))
		fmt.Println(fullSQL)
		fmt.Println(strings.Repeat("-", 80) + "\n")
		return fullSQL, nil
	}

	// Execute ALTER statements
	successCount := 0
	for _, stmt := range alterStatements {
		result := dbc.DB.Exec(stmt)
		if result.Error != nil {
			log.WithError(result.Error).WithField("statement", stmt).Error("failed to execute ALTER statement")
			continue
		}
		successCount++
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":      tableName,
		"statements": len(alterStatements),
		"successful": successCount,
		"elapsed":    elapsed,
	}).Info("updated partitioned table schema")

	return fullSQL, nil
}

// getCurrentColumns retrieves the current column schema from the database
// getCurrentIndexes retrieves the current indexes from the database
func getCurrentIndexes(dbc *db.DB, tableName string) ([]indexInfo, error) {
	type indexRow struct {
		IndexName string
		IsUnique  bool
		Column    string
	}

	var rows []indexRow

	query := `
		SELECT
			i.indexname AS index_name,
			ix.indisunique AS is_unique,
			a.attname AS column
		FROM pg_indexes i
		JOIN pg_class c ON c.relname = i.indexname
		JOIN pg_index ix ON ix.indexrelid = c.oid
		JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = ANY(ix.indkey)
		WHERE i.schemaname = 'public'
			AND i.tablename = $1
		ORDER BY i.indexname, a.attnum
	`

	result := dbc.DB.Raw(query, tableName).Scan(&rows)
	if result.Error != nil {
		return nil, result.Error
	}

	// Group by index name
	indexMap := make(map[string]*indexInfo)
	for _, row := range rows {
		if idx, exists := indexMap[row.IndexName]; exists {
			idx.Columns = append(idx.Columns, row.Column)
		} else {
			indexMap[row.IndexName] = &indexInfo{
				IndexName: row.IndexName,
				IsUnique:  row.IsUnique,
				Columns:   []string{row.Column},
			}
		}
	}

	var indexes []indexInfo
	for _, idx := range indexMap {
		indexes = append(indexes, *idx)
	}

	return indexes, nil
}

// getPartitionColumns retrieves the partition key columns for a table
func getPartitionColumns(dbc *db.DB, tableName string) ([]string, error) {
	var columns []string

	query := `
		SELECT a.attname
		FROM pg_class c
		JOIN pg_partitioned_table pt ON pt.partrelid = c.oid
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(pt.partattrs)
		WHERE c.relname = $1
			AND c.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
		ORDER BY array_position(pt.partattrs, a.attnum)
	`

	result := dbc.DB.Raw(query, tableName).Scan(&columns)
	if result.Error != nil {
		return nil, result.Error
	}

	return columns, nil
}

// normalizeDataType normalizes data type strings for comparison
func normalizeDataType(dataType string) string {
	// Convert to lowercase and remove common variations
	normalized := strings.ToLower(strings.TrimSpace(dataType))

	// Handle common type mappings
	replacements := map[string]string{
		"character varying":           "varchar",
		"integer":                     "int",
		"bigint":                      "int8",
		"smallint":                    "int2",
		"boolean":                     "bool",
		"timestamp without time zone": "timestamp",
		"timestamp with time zone":    "timestamptz",
		"double precision":            "float8",
		"real":                        "float4",
		"character":                   "char",
		"time without time zone":      "time",
		"time with time zone":         "timetz",
	}

	for old, new := range replacements {
		if strings.Contains(normalized, old) {
			normalized = strings.ReplaceAll(normalized, old, new)
		}
	}

	return normalized
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

	// Get only attached partitions for removal (can only detach what's attached)
	partitions, err := GetPartitionsForRemoval(dbc, tableName, retentionDays, true)
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
