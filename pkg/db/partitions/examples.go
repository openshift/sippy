package partitions

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

// ExampleListPartitionedTables demonstrates how to list all partitioned tables
//
// Usage:
//
//	ExampleListPartitionedTables(dbc)
func ExampleListPartitionedTables(dbc *db.DB) {
	tables, err := ListPartitionedTables(dbc)
	if err != nil {
		log.WithError(err).Error("failed to list partitioned tables")
		return
	}

	fmt.Printf("Found %d partitioned tables:\n", len(tables))
	for _, t := range tables {
		fmt.Printf("  %s: %d partitions, Strategy: %s\n",
			t.TableName, t.PartitionCount, t.PartitionStrategy)
	}
}

// ExampleListPartitions demonstrates how to list partitions for a table
// If retentionDays > 0, only shows partitions older than that value
// If retentionDays <= 0, shows all partitions
//
// Usage:
//
//	ExampleListPartitions(dbc, "test_analysis_by_job_by_dates", 180)  // Show partitions older than 180 days
//	ExampleListPartitions(dbc, "test_analysis_by_job_by_dates", 0)    // Show all partitions
func ExampleListPartitions(dbc *db.DB, tableName string, retentionDays int) {
	partitions, err := ListTablePartitions(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to list partitions")
		return
	}

	if retentionDays > 0 {
		fmt.Printf("Partitions older than %d days for %s:\n", retentionDays, tableName)
	} else {
		fmt.Printf("All partitions for %s:\n", tableName)
	}

	for _, p := range partitions {
		if p.Age > retentionDays || retentionDays < 1 {
			fmt.Printf("  %s - Date: %s, Age: %d days, Size: %s\n",
				p.TableName, p.PartitionDate.Format("2006-01-02"), p.Age, p.SizePretty)
		}
	}
}

// ExampleGetStats demonstrates how to get partition statistics
//
// Usage:
//
//	ExampleGetStats(dbc, "test_analysis_by_job_by_dates")
func ExampleGetStats(dbc *db.DB, tableName string) {
	stats, err := GetPartitionStats(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to get stats")
		return
	}

	fmt.Printf("\nPartition Statistics for %s:\n", tableName)
	fmt.Printf("  Total Partitions: %d\n", stats.TotalPartitions)
	fmt.Printf("  Total Size: %s\n", stats.TotalSizePretty)
	fmt.Printf("  Average Size: %s\n", stats.AvgSizePretty)
	fmt.Printf("  Date Range: %s to %s\n",
		stats.OldestDate.Format("2006-01-02"),
		stats.NewestDate.Format("2006-01-02"))
}

// ExampleComparePartitionStats demonstrates comparing attached vs detached partition statistics
//
// Usage:
//
//	ExampleComparePartitionStats(dbc, "test_analysis_by_job_by_dates")
func ExampleComparePartitionStats(dbc *db.DB, tableName string) {
	fmt.Printf("\n=== Partition Statistics Comparison for %s ===\n", tableName)

	// Get all partition stats
	allStats, err := GetPartitionStats(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to get all partition stats")
		return
	}

	// Get attached partition stats
	attachedStats, err := GetAttachedPartitionStats(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to get attached partition stats")
		return
	}

	// Get detached partition stats
	detachedStats, err := GetDetachedPartitionStats(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to get detached partition stats")
		return
	}

	fmt.Printf("\nAll Partitions (Attached + Detached):\n")
	fmt.Printf("  Total: %d partitions (%s)\n", allStats.TotalPartitions, allStats.TotalSizePretty)

	fmt.Printf("\nAttached Partitions:\n")
	fmt.Printf("  Total: %d partitions (%s)\n", attachedStats.TotalPartitions, attachedStats.TotalSizePretty)
	if attachedStats.TotalPartitions > 0 {
		fmt.Printf("  Range: %s to %s\n",
			attachedStats.OldestDate.Format("2006-01-02"),
			attachedStats.NewestDate.Format("2006-01-02"))
	}

	fmt.Printf("\nDetached Partitions:\n")
	fmt.Printf("  Total: %d partitions (%s)\n", detachedStats.TotalPartitions, detachedStats.TotalSizePretty)
	if detachedStats.TotalPartitions > 0 {
		fmt.Printf("  Range: %s to %s\n",
			detachedStats.OldestDate.Format("2006-01-02"),
			detachedStats.NewestDate.Format("2006-01-02"))
	}

	// Calculate percentages
	if allStats.TotalPartitions > 0 {
		attachedPct := float64(attachedStats.TotalPartitions) / float64(allStats.TotalPartitions) * 100
		detachedPct := float64(detachedStats.TotalPartitions) / float64(allStats.TotalPartitions) * 100
		fmt.Printf("\nDistribution:\n")
		fmt.Printf("  Attached: %.1f%%\n", attachedPct)
		fmt.Printf("  Detached: %.1f%%\n", detachedPct)
	}
}

// ExampleCheckRetentionPolicy demonstrates how to check what a retention policy would affect
//
// Usage:
//
//	ExampleCheckRetentionPolicy(dbc, "test_analysis_by_job_by_dates", 180)
func ExampleCheckRetentionPolicy(dbc *db.DB, tableName string, retentionDays int) {
	// First validate the policy
	if err := ValidateRetentionPolicy(dbc, tableName, retentionDays); err != nil {
		log.WithError(err).Error("retention policy validation failed")
		return
	}

	// Get summary of what would be affected (all partitions)
	summary, err := GetRetentionSummary(dbc, tableName, retentionDays, false)
	if err != nil {
		log.WithError(err).Error("failed to get retention summary")
		return
	}

	fmt.Printf("\nRetention Policy Analysis for %s (%d days):\n", tableName, retentionDays)
	fmt.Printf("  Cutoff Date: %s\n", summary.CutoffDate.Format("2006-01-02"))
	fmt.Printf("  Partitions to Remove: %d\n", summary.PartitionsToRemove)
	fmt.Printf("  Storage to Reclaim: %s\n", summary.StoragePretty)
	if summary.PartitionsToRemove > 0 {
		fmt.Printf("  Oldest: %s\n", summary.OldestPartition)
		fmt.Printf("  Newest: %s\n", summary.NewestPartition)
	}

	// Get detailed list of partitions that would be removed (both attached and detached)
	partitions, err := GetPartitionsForRemoval(dbc, tableName, retentionDays, false)
	if err != nil {
		log.WithError(err).Error("failed to get partitions for removal")
		return
	}

	if len(partitions) > 0 {
		fmt.Printf("\nPartitions that would be removed (showing first 10):\n")
		for i, p := range partitions {
			if i < 10 {
				fmt.Printf("  %s - %s ago, Size: %s\n",
					p.TableName, p.PartitionDate.Format("2006-01-02"), p.SizePretty)
			}
		}
		if len(partitions) > 10 {
			fmt.Printf("  ... and %d more\n", len(partitions)-10)
		}
	}
}

// ExampleAgeGroupAnalysis demonstrates how to analyze partitions by age
//
// Usage:
//
//	ExampleAgeGroupAnalysis(dbc, "test_analysis_by_job_by_dates")
func ExampleAgeGroupAnalysis(dbc *db.DB, tableName string) {
	groups, err := GetPartitionsByAgeGroup(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to get age groups")
		return
	}

	fmt.Printf("\nPartitions by Age Group for %s:\n", tableName)
	for _, group := range groups {
		fmt.Printf("  %s: %d partitions, %s (%.2f%%)\n",
			group["age_bucket"],
			group["partition_count"],
			group["total_size"],
			group["percentage"])
	}
}

// ExampleMonthlyAnalysis demonstrates how to analyze partitions by month
//
// Usage:
//
//	ExampleMonthlyAnalysis(dbc, "test_analysis_by_job_by_dates")
func ExampleMonthlyAnalysis(dbc *db.DB, tableName string) {
	months, err := GetPartitionsByMonth(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to get monthly breakdown")
		return
	}

	fmt.Printf("\nPartitions by Month for %s (recent):\n", tableName)
	for i, month := range months {
		if i < 6 { // Show last 6 months
			fmt.Printf("  %v: %d partitions, Total: %s, Avg: %s\n",
				month["month"],
				month["partition_count"],
				month["total_size"],
				month["avg_partition_size"])
		}
	}
}

// ExampleDryRunCleanup demonstrates a dry-run cleanup operation
//
// Usage:
//
//	ExampleDryRunCleanup(dbc, "test_analysis_by_job_by_dates", 180)
func ExampleDryRunCleanup(dbc *db.DB, tableName string, retentionDays int) {
	fmt.Printf("\n=== DRY RUN: Partition Cleanup for %s (%d day retention) ===\n", tableName, retentionDays)

	// Validate policy
	if err := ValidateRetentionPolicy(dbc, tableName, retentionDays); err != nil {
		log.WithError(err).Error("retention policy failed validation")
		return
	}

	// Get summary (all partitions, matching DropOldPartitions behavior)
	summary, err := GetRetentionSummary(dbc, tableName, retentionDays, false)
	if err != nil {
		log.WithError(err).Error("failed to get summary")
		return
	}

	if summary.PartitionsToRemove == 0 {
		fmt.Println("No partitions to delete")
		return
	}

	fmt.Printf("Would delete %d partitions, reclaiming %s\n",
		summary.PartitionsToRemove, summary.StoragePretty)

	// Perform dry run
	dropped, err := DropOldPartitions(dbc, tableName, retentionDays, true) // true = dry run
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}

	fmt.Printf("Dry run completed: would drop %d partitions\n", dropped)
}

// ExampleDetachedPartitions demonstrates working with detached partitions for a table
//
// Usage:
//
//	ExampleDetachedPartitions(dbc, "test_analysis_by_job_by_dates")
func ExampleDetachedPartitions(dbc *db.DB, tableName string) {
	fmt.Printf("\n=== Detached Partitions for %s ===\n", tableName)

	// List detached partitions
	detached, err := ListDetachedPartitions(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to list detached partitions")
		return
	}

	if len(detached) == 0 {
		fmt.Println("No detached partitions found")
		return
	}

	fmt.Printf("Found %d detached partitions:\n", len(detached))
	for i, p := range detached {
		if i < 5 {
			fmt.Printf("  %s - Date: %s, Size: %s\n",
				p.TableName, p.PartitionDate.Format("2006-01-02"), p.SizePretty)
		}
	}

	// Get statistics about detached partitions
	stats, err := GetDetachedPartitionStats(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to get detached stats")
		return
	}

	fmt.Printf("\nDetached Partition Statistics:\n")
	fmt.Printf("  Total: %d partitions (%s)\n", stats.TotalPartitions, stats.TotalSizePretty)
	if stats.TotalPartitions > 0 {
		fmt.Printf("  Range: %s to %s\n",
			stats.OldestDate.Format("2006-01-02"),
			stats.NewestDate.Format("2006-01-02"))
	}
}

// ExampleAttachedPartitions demonstrates working with attached partitions for a table
//
// Usage:
//
//	ExampleAttachedPartitions(dbc, "test_analysis_by_job_by_dates")
func ExampleAttachedPartitions(dbc *db.DB, tableName string) {
	fmt.Printf("\n=== Attached Partitions for %s ===\n", tableName)

	// List attached partitions
	attached, err := ListAttachedPartitions(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to list attached partitions")
		return
	}

	if len(attached) == 0 {
		fmt.Println("No attached partitions found")
		return
	}

	fmt.Printf("Found %d attached partitions:\n", len(attached))
	for i, p := range attached {
		if i < 10 {
			fmt.Printf("  %s - Date: %s, Age: %d days, Size: %s\n",
				p.TableName, p.PartitionDate.Format("2006-01-02"), p.Age, p.SizePretty)
		}
	}

	if len(attached) > 10 {
		fmt.Printf("  ... and %d more\n", len(attached)-10)
	}

	// Calculate total size
	var totalSize int64
	for _, p := range attached {
		totalSize += p.SizeBytes
	}

	fmt.Printf("\nAttached Partition Summary:\n")
	fmt.Printf("  Total: %d partitions\n", len(attached))
	fmt.Printf("  Total Size: %d bytes\n", totalSize)
	if len(attached) > 0 {
		fmt.Printf("  Range: %s to %s\n",
			attached[0].PartitionDate.Format("2006-01-02"),
			attached[len(attached)-1].PartitionDate.Format("2006-01-02"))
	}
}

// ExampleDropOldDetachedPartitions demonstrates dropping old detached partitions
//
// Usage:
//
//	ExampleDropOldDetachedPartitions(dbc, "test_analysis_by_job_by_dates", 180)
func ExampleDropOldDetachedPartitions(dbc *db.DB, tableName string, retentionDays int) {
	fmt.Printf("\n=== Drop Old Detached Partitions for %s (%d days) ===\n", tableName, retentionDays)

	// 1. Check what detached partitions exist
	detached, err := ListDetachedPartitions(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to list detached partitions")
		return
	}

	if len(detached) == 0 {
		fmt.Println("No detached partitions found")
		return
	}

	fmt.Printf("Found %d detached partitions\n", len(detached))

	// 2. Show which ones would be dropped
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)
	fmt.Printf("Cutoff date: %s\n", cutoffDate.Format("2006-01-02"))

	toRemove := 0
	var totalSize int64
	for _, p := range detached {
		if p.PartitionDate.Before(cutoffDate) {
			toRemove++
			totalSize += p.SizeBytes
			if toRemove <= 5 {
				fmt.Printf("  Would drop: %s (Age: %d days, Size: %s)\n",
					p.TableName, p.Age, p.SizePretty)
			}
		}
	}

	if toRemove > 5 {
		fmt.Printf("  ... and %d more\n", toRemove-5)
	}

	if toRemove == 0 {
		fmt.Println("No detached partitions older than retention period")
		return
	}

	fmt.Printf("\nTotal to remove: %d partitions\n", toRemove)

	// 3. Dry run
	fmt.Println("\nRunning dry run...")
	dropped, err := DropOldDetachedPartitions(dbc, tableName, retentionDays, true)
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}

	fmt.Printf("Dry run completed: would drop %d detached partitions\n", dropped)

	// 4. Actual drop (commented out for safety)
	// fmt.Println("\nActual drop (uncomment to execute):")
	// dropped, err = DropOldDetachedPartitions(dbc, tableName, retentionDays, false)
	// if err != nil {
	//     log.WithError(err).Error("drop failed")
	//     return
	// }
	// fmt.Printf("Dropped %d detached partitions\n", dropped)
}

// ExampleDetachWorkflow demonstrates the detach/archive workflow
//
// Usage:
//
//	ExampleDetachWorkflow(dbc, "test_analysis_by_job_by_dates", 180)
func ExampleDetachWorkflow(dbc *db.DB, tableName string, retentionDays int) {
	fmt.Printf("\n=== Detach Workflow for %s (%d days) ===\n", tableName, retentionDays)

	// 1. Check what would be detached (only attached partitions)
	summary, err := GetRetentionSummary(dbc, tableName, retentionDays, true)
	if err != nil {
		log.WithError(err).Error("failed to get summary")
		return
	}

	fmt.Printf("1. Would detach %d partitions (%s)\n",
		summary.PartitionsToRemove, summary.StoragePretty)

	// 2. Detach partitions (dry run)
	detached, err := DetachOldPartitions(dbc, tableName, retentionDays, true)
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}

	fmt.Printf("2. Dry run: would detach %d partitions\n", detached)

	// 3. Actual detach (commented out - requires admin)
	// detached, err = DetachOldPartitions(dbc, tableName, retentionDays, false)
	// fmt.Printf("3. Detached %d partitions\n", detached)

	// 4. Check detached partitions
	fmt.Println("\n4. After detachment, you can:")
	fmt.Println("   - Archive to S3 using external scripts")
	fmt.Println("   - Compress and store offline")
	fmt.Println("   - Query detached tables directly if needed")
	fmt.Println("   - Reattach if data is needed again")
	fmt.Println("   - Drop when ready to free storage")
}

// ExampleReattachPartition demonstrates reattaching a detached partition
//
// Usage:
//
//	ExampleReattachPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29")
func ExampleReattachPartition(dbc *db.DB, partitionName string) {
	fmt.Printf("\n=== Reattach Partition: %s ===\n", partitionName)

	// 1. Check if partition is attached
	isAttached, err := IsPartitionAttached(dbc, partitionName)
	if err != nil {
		log.WithError(err).Error("failed to check partition status")
		return
	}

	fmt.Printf("1. Partition attached: %v\n", isAttached)

	if isAttached {
		fmt.Println("Partition is already attached, no action needed")
		return
	}

	// 2. Reattach (dry run)
	err = ReattachPartition(dbc, partitionName, true)
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}

	fmt.Println("2. Dry run successful")

	// 3. Actual reattach (commented out - requires admin)
	// err = ReattachPartition(dbc, partitionName, false)
	// if err != nil {
	//     log.WithError(err).Error("reattach failed")
	//     return
	// }
	// fmt.Println("3. Partition reattached successfully")
}

// ExampleCreateMissingPartitions demonstrates creating missing partitions for a date range
//
// Usage:
//
//	ExampleCreateMissingPartitions(dbc, "test_analysis_by_job_by_dates", "2024-01-01", "2024-01-31")
func ExampleCreateMissingPartitions(dbc *db.DB, tableName, startDateStr, endDateStr string) {
	fmt.Printf("\n=== Create Missing Partitions for %s ===\n", tableName)

	// Parse dates
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		log.WithError(err).Error("failed to parse start date")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		log.WithError(err).Error("failed to parse end date")
		return
	}

	fmt.Printf("Date range: %s to %s\n", startDateStr, endDateStr)

	// Calculate expected number of partitions
	days := int(endDate.Sub(startDate).Hours()/24) + 1
	fmt.Printf("Expected partitions: %d (one per day)\n", days)

	// Check current partitions
	existing, err := ListTablePartitions(dbc, tableName)
	if err != nil {
		log.WithError(err).Error("failed to list existing partitions")
		return
	}
	fmt.Printf("Existing partitions: %d\n", len(existing))

	// Dry run to see what would be created
	fmt.Println("\nDry run...")
	created, err := CreateMissingPartitions(dbc, tableName, startDate, endDate, true)
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}

	if created == 0 {
		fmt.Println("All partitions already exist - no action needed")
		return
	}

	fmt.Printf("Would create %d missing partitions\n", created)

	// Actual creation (commented out for safety)
	// fmt.Println("\nCreating partitions...")
	// created, err = CreateMissingPartitions(dbc, tableName, startDate, endDate, false)
	// if err != nil {
	//     log.WithError(err).Error("creation failed")
	//     return
	// }
	// fmt.Printf("Successfully created %d partitions\n", created)
}

// ExampleCreatePartitionedTable demonstrates creating a new partitioned table from a GORM model
//
// Usage:
//
//	config := partitions.NewRangePartitionConfig("created_at")
//	ExampleCreatePartitionedTable(dbc, &models.MyModel{}, "my_partitioned_table", config)
func ExampleCreatePartitionedTable(dbc *db.DB, model interface{}, tableName string, config PartitionConfig) {
	fmt.Printf("\n=== Create Partitioned Table: %s ===\n", tableName)
	fmt.Printf("Strategy: %s\n", config.Strategy)
	fmt.Printf("Partition by: %s\n", strings.Join(config.Columns, ", "))

	// Dry run to see the SQL that would be executed
	fmt.Println("\nDry run - SQL that would be executed:")
	sql, err := CreatePartitionedTable(dbc, model, tableName, config, true)
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}

	if sql == "" {
		fmt.Println("Table already exists - no action needed")
		return
	}

	// Actual creation (commented out for safety)
	// fmt.Println("\nCreating partitioned table...")
	// _, err = CreatePartitionedTable(dbc, model, tableName, config, false)
	// if err != nil {
	//     log.WithError(err).Error("table creation failed")
	//     return
	// }
	// fmt.Printf("Successfully created partitioned table: %s\n", tableName)
	//
	// // For RANGE partitions, create partitions for your date range
	// if config.Strategy == db.PartitionStrategyRange {
	//     startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	//     endDate := time.Now()
	//     created, err := CreateMissingPartitions(dbc, tableName, startDate, endDate, false)
	//     if err != nil {
	//         log.WithError(err).Error("partition creation failed")
	//         return
	//     }
	//     fmt.Printf("Created %d partitions\n", created)
	// }
	//
	// // For HASH partitions, create the required number of partitions
	// if config.Strategy == db.PartitionStrategyHash {
	//     for i := 0; i < config.Modulus; i++ {
	//         partName := fmt.Sprintf("%s_%d", tableName, i)
	//         sql := fmt.Sprintf("CREATE TABLE %s PARTITION OF %s FOR VALUES WITH (MODULUS %d, REMAINDER %d)",
	//             partName, tableName, config.Modulus, i)
	//         if err := dbc.DB.Exec(sql).Error; err != nil {
	//             log.WithError(err).Errorf("failed to create partition %s", partName)
	//         }
	//     }
	// }
}

// ExampleUpdatePartitionedTable demonstrates updating an existing partitioned table schema
//
// Usage:
//
//	ExampleUpdatePartitionedTable(dbc, &models.MyModel{}, "my_partitioned_table")
func ExampleUpdatePartitionedTable(dbc *db.DB, model interface{}, tableName string) {
	fmt.Printf("\n=== Update Partitioned Table Schema: %s ===\n", tableName)

	// Dry run to see what changes would be made
	fmt.Println("\nDry run - checking for schema changes:")
	sql, err := UpdatePartitionedTable(dbc, model, tableName, true)
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}

	if sql == "" {
		fmt.Println("Schema is up to date - no changes needed")
		return
	}

	fmt.Printf("\nChanges detected:\n%s\n", sql)

	// Actual update (commented out for safety)
	// fmt.Println("\nApplying schema changes...")
	// _, err = UpdatePartitionedTable(dbc, model, tableName, false)
	// if err != nil {
	//     log.WithError(err).Error("schema update failed")
	//     return
	// }
	// fmt.Printf("Successfully updated table schema: %s\n", tableName)
}

// ExampleWorkflowForAnyTable demonstrates managing partitions for any table
//
// Usage:
//
//	ExampleWorkflowForAnyTable(dbc)
func ExampleWorkflowForAnyTable(dbc *db.DB) {
	fmt.Println("=== Managing Partitions for Any Table ===")

	// 1. List all partitioned tables
	fmt.Println("\n1. Discovering partitioned tables:")
	tables, err := ListPartitionedTables(dbc)
	if err != nil {
		log.WithError(err).Error("failed to list partitioned tables")
		return
	}

	for _, table := range tables {
		fmt.Printf("   - %s: %d partitions (%s)\n",
			table.TableName, table.PartitionCount, table.PartitionStrategy)
	}

	// 2. For each table, analyze retention
	fmt.Println("\n2. Analyzing retention policies:")
	for _, table := range tables {
		fmt.Printf("\nTable: %s\n", table.TableName)

		// Get current stats
		stats, err := GetPartitionStats(dbc, table.TableName)
		if err != nil {
			log.WithError(err).WithField("table", table.TableName).Error("failed to get stats")
			continue
		}

		fmt.Printf("  Total: %d partitions (%s)\n",
			stats.TotalPartitions, stats.TotalSizePretty)
		fmt.Printf("  Range: %s to %s\n",
			stats.OldestDate.Format("2006-01-02"),
			stats.NewestDate.Format("2006-01-02"))

		// Check 180-day retention policy (all partitions)
		summary, err := GetRetentionSummary(dbc, table.TableName, 180, false)
		if err != nil {
			log.WithError(err).WithField("table", table.TableName).Error("failed to get summary")
			continue
		}

		if summary.PartitionsToRemove > 0 {
			fmt.Printf("  180-day policy: Would remove %d partitions (%s)\n",
				summary.PartitionsToRemove, summary.StoragePretty)
		} else {
			fmt.Println("  180-day policy: No partitions to remove")
		}
	}
}

// ExampleCompleteWorkflow demonstrates a complete partition management workflow for a specific table
//
// Usage:
//
//	ExampleCompleteWorkflow(dbc, "test_analysis_by_job_by_dates")
func ExampleCompleteWorkflow(dbc *db.DB, tableName string) {
	fmt.Printf("=== Partition Management Workflow for %s ===\n", tableName)

	// 1. Get current state
	fmt.Println("\n1. Current State:")
	ExampleGetStats(dbc, tableName)

	// 2. Analyze by age
	fmt.Println("\n2. Age Distribution:")
	ExampleAgeGroupAnalysis(dbc, tableName)

	// 3. Check various retention policies
	for _, days := range []int{90, 180, 365} {
		fmt.Printf("\n3. Analyzing %d-day retention policy:\n", days)
		ExampleCheckRetentionPolicy(dbc, tableName, days)
	}

	// 4. Recommended: 180-day retention dry run
	fmt.Println("\n4. Recommended Policy (180 days):")
	ExampleDryRunCleanup(dbc, tableName, 180)

	// 5. Check for detached partitions
	fmt.Println("\n5. Detached Partitions:")
	ExampleDetachedPartitions(dbc, tableName)

	fmt.Println("\n=== Workflow Complete ===")
	fmt.Println("Options for cleanup:")
	fmt.Printf("  1. DROP immediately:\n")
	fmt.Printf("     dropped, err := partitions.DropOldPartitions(dbc, \"%s\", 180, false)\n", tableName)
	fmt.Printf("  2. DETACH for archival:\n")
	fmt.Printf("     detached, err := partitions.DetachOldPartitions(dbc, \"%s\", 180, false)\n", tableName)
	fmt.Println("     // Archive detached partitions to S3")
	fmt.Println("     // Drop detached partitions when archived")
}
