package partitions_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
)

// TestPartitionLifecycle verifies partition creation, detachment, and dropping.
func TestPartitionLifecycle(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)

	// Cleanup function to remove test data
	t.Cleanup(func() {
		// Clean up test prow jobs, runs, and tests
		dbc.DB.Unscoped().Where("name LIKE ?", "test-partition-%").Delete(&models.ProwJob{})
		dbc.DB.Unscoped().Where("name LIKE ?", "test-partition-%").Delete(&models.Test{})
		// Note: CASCADE delete should clean up related records
	})

	t.Run("EnsurePartitions creates missing partitions", func(t *testing.T) {
		releases := []string{"4.17", "4.18"}
		startDate := time.Now().AddDate(0, 0, -7) // 7 days ago
		endDate := time.Now().AddDate(0, 0, 1)    // Tomorrow

		count, err := dbc.EnsurePartitions(releases, startDate, endDate, false)
		require.NoError(t, err, "EnsurePartitions should succeed")
		assert.Greater(t, count, 0, "should create at least one partition")

		// Verify partitions were created by checking pg_inherits
		var partitionCount int64
		err = dbc.DB.Raw(`
			SELECT COUNT(*)
			FROM pg_inherits
			JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
			JOIN pg_class child ON pg_inherits.inhrelid = child.oid
			WHERE parent.relname IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
		`).Scan(&partitionCount).Error
		require.NoError(t, err)
		assert.Greater(t, partitionCount, int64(0), "should have created partitions")
	})

	t.Run("DetachOldPartitions detaches partitions older than retention period", func(t *testing.T) {
		// Create partitions for old data (105 days ago) so they exceed the 100-day retention
		releases := []string{"4.17"}
		oldDate := time.Now().AddDate(0, 0, -105) // 105 days ago
		endDate := oldDate.AddDate(0, 0, 2)       // 2-day range

		count, err := dbc.EnsurePartitions(releases, oldDate, endDate, false)
		require.NoError(t, err, "EnsurePartitions for old data should succeed")
		require.Greater(t, count, 0, "should create old partitions for testing")

		// Insert test data to ensure partitions are populated
		prowJob := models.ProwJob{Name: "test-partition-detach-job"}
		err = dbc.DB.Create(&prowJob).Error
		require.NoError(t, err, "should create test prow job")

		prowJobRun := models.ProwJobRun{
			ProwJobID:      prowJob.ID,
			ProwJobRelease: "4.17",
			Timestamp:      oldDate,
			Succeeded:      true,
		}
		err = dbc.DB.Create(&prowJobRun).Error
		require.NoError(t, err, "should create test prow job run")

		test := models.Test{Name: "test-partition-detach-test"}
		err = dbc.DB.Create(&test).Error
		require.NoError(t, err, "should create test")

		prowJobRunTest := models.ProwJobRunTest{
			ProwJobRunID:        prowJobRun.ID,
			ProwJobID:           prowJob.ID,
			ProwJobRunTimestamp: oldDate,
			ProwJobRunRelease:   "4.17",
			TestID:              test.ID,
			Status:              12, // Success
		}
		err = dbc.DB.Create(&prowJobRunTest).Error
		require.NoError(t, err, "should create test data in old partition")

		// Detach partitions older than 100 days
		detachedCount, err := dbc.DetachOldPartitions(100, false)
		require.NoError(t, err, "DetachOldPartitions should succeed")
		assert.Greater(t, detachedCount, 0, "should detach at least one partition")

		// Verify partitions were detached by checking they no longer inherit from parent
		var attachedPartitions []string
		err = dbc.DB.Raw(`
			SELECT child.relname
			FROM pg_inherits
			JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
			JOIN pg_class child ON pg_inherits.inhrelid = child.oid
			WHERE parent.relname IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
		`).Scan(&attachedPartitions).Error
		require.NoError(t, err)

		// Old partitions should not be in the attached list
		for _, partition := range attachedPartitions {
			assert.NotContains(t, partition, oldDate.Format("2006_01_02"),
				"detached partition should not appear in attached partitions list")
		}
	})

	t.Run("DropDetachedPartitions drops detached partitions", func(t *testing.T) {
		// Create very old partitions (115 days ago) that will be detached and then dropped
		releases := []string{"4.17"}
		veryOldDate := time.Now().AddDate(0, 0, -115) // 115 days ago
		endDate := veryOldDate.AddDate(0, 0, 2)       // 2-day range

		count, err := dbc.EnsurePartitions(releases, veryOldDate, endDate, false)
		require.NoError(t, err, "EnsurePartitions for very old data should succeed")
		require.Greater(t, count, 0, "should create very old partitions for testing")

		// Get partition names before detachment
		var partitionsBefore []string
		err = dbc.DB.Raw(`
			SELECT tablename
			FROM pg_tables
			WHERE tablename LIKE '%` + veryOldDate.Format("2006_01_02") + `%'
			  AND schemaname = 'public'
		`).Scan(&partitionsBefore).Error
		require.NoError(t, err)
		require.Greater(t, len(partitionsBefore), 0, "should have partitions to test with")

		// Detach partitions older than 110 days (so our 115-day-old partitions get detached)
		detachedCount, err := dbc.DetachOldPartitions(110, false)
		require.NoError(t, err, "DetachOldPartitions should succeed")
		require.Greater(t, detachedCount, 0, "should detach partitions")

		// Now drop detached partitions older than 110 days
		droppedCount, err := dbc.DropDetachedPartitions(110, false)
		require.NoError(t, err, "DropDetachedPartitions should succeed")
		assert.Greater(t, droppedCount, 0, "should drop at least one detached partition")

		// Verify partitions were dropped
		var partitionsAfter []string
		err = dbc.DB.Raw(`
			SELECT tablename
			FROM pg_tables
			WHERE tablename LIKE '%` + veryOldDate.Format("2006_01_02") + `%'
			  AND schemaname = 'public'
		`).Scan(&partitionsAfter).Error
		require.NoError(t, err)

		// Should have fewer partitions after dropping
		assert.Less(t, len(partitionsAfter), len(partitionsBefore),
			"should have fewer partitions after dropping")
	})

	t.Run("CleanupPartitions orchestrates detach and drop", func(t *testing.T) {
		// Create partitions at different ages:
		// 1. Recent (within 100 days) - should NOT be detached
		// 2. Old (105 days) - should be detached but NOT dropped
		// 3. Very old (115 days) - should be dropped
		releases := []string{"4.18"}

		// Recent data
		recentDate := time.Now().AddDate(0, 0, -50)
		count, err := dbc.EnsurePartitions(releases, recentDate, recentDate.AddDate(0, 0, 1), false)
		require.NoError(t, err)
		require.Greater(t, count, 0, "should create recent partitions")

		// Old data (will be detached)
		oldDate := time.Now().AddDate(0, 0, -105)
		count, err = dbc.EnsurePartitions(releases, oldDate, oldDate.AddDate(0, 0, 1), false)
		require.NoError(t, err)
		require.Greater(t, count, 0, "should create old partitions")

		// Very old data (will be dropped)
		veryOldDate := time.Now().AddDate(0, 0, -115)
		count, err = dbc.EnsurePartitions(releases, veryOldDate, veryOldDate.AddDate(0, 0, 1), false)
		require.NoError(t, err)
		require.Greater(t, count, 0, "should create very old partitions")

		// First detach the very old partitions so they can be dropped
		_, err = dbc.DetachOldPartitions(110, false)
		require.NoError(t, err)

		// Diagnostic: Check partition bounds before cleanup
		type partitionDiagnostic struct {
			TableName     string
			BoundRaw      string
			ExtractedDate *time.Time
			Age           *int
		}
		var diagnostics []partitionDiagnostic
		err = dbc.DB.Raw(`
			WITH RECURSIVE partition_tree AS (
				SELECT
					c.oid,
					c.relname AS table_name,
					0 AS level
				FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE c.relname IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
				  AND n.nspname = 'public'

				UNION ALL

				SELECT
					child.oid,
					child.relname AS table_name,
					pt.level + 1
				FROM partition_tree pt
				JOIN pg_class parent ON parent.relname = pt.table_name
				JOIN pg_inherits i ON i.inhparent = parent.oid
				JOIN pg_class child ON child.oid = i.inhrelid
			)
			SELECT
				pt.table_name,
				pg_get_expr(c.relpartbound, c.oid) AS bound_raw,
				TO_DATE(
					substring(pg_get_expr(c.relpartbound, c.oid) FROM 'FROM \(''(\d{4}-\d{2}-\d{2})'),
					'YYYY-MM-DD'
				) AS extracted_date,
				(CURRENT_DATE - TO_DATE(
					substring(pg_get_expr(c.relpartbound, c.oid) FROM 'FROM \(''(\d{4}-\d{2}-\d{2})'),
					'YYYY-MM-DD'
				))::INT AS age
			FROM partition_tree pt
			JOIN pg_class c ON c.relname = pt.table_name
			WHERE NOT EXISTS (
				SELECT 1 FROM pg_partitioned_table pp WHERE pp.partrelid = c.oid
			)
			AND pt.level > 0
			AND substring(pg_get_expr(c.relpartbound, c.oid) FROM 'FROM \(''(\d{4}-\d{2}-\d{2})') IS NOT NULL
			AND pt.table_name LIKE '%` + recentDate.Format("2006_01_02") + `%'
			ORDER BY pt.table_name
		`).Scan(&diagnostics).Error
		require.NoError(t, err)

		t.Logf("Recent partitions before cleanup (should be ~50 days old):")
		for _, d := range diagnostics {
			t.Logf("  %s: bound=%q, extracted_date=%v, age=%v",
				d.TableName, d.BoundRaw, d.ExtractedDate, d.Age)
		}
		require.Greater(t, len(diagnostics), 0, "should find recent partitions before cleanup")

		// Run CleanupPartitions
		detached, dropped, err := dbc.CleanupPartitions(false)
		require.NoError(t, err, "CleanupPartitions should succeed")

		// Verify cleanup occurred
		t.Logf("Cleanup results: detached=%d, dropped=%d", detached, dropped)
		assert.Greater(t, dropped, 0, "should drop very old detached partitions")
		assert.GreaterOrEqual(t, detached, 0, "may detach old partitions")

		// Check what happened to recent partitions after cleanup
		var recentAfterCleanup []partitionDiagnostic
		err = dbc.DB.Raw(`
			WITH RECURSIVE partition_tree AS (
				SELECT
					c.oid,
					c.relname AS table_name,
					0 AS level
				FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE c.relname IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
				  AND n.nspname = 'public'

				UNION ALL

				SELECT
					child.oid,
					child.relname AS table_name,
					pt.level + 1
				FROM partition_tree pt
				JOIN pg_class parent ON parent.relname = pt.table_name
				JOIN pg_inherits i ON i.inhparent = parent.oid
				JOIN pg_class child ON child.oid = i.inhrelid
			)
			SELECT
				pt.table_name,
				pg_get_expr(c.relpartbound, c.oid) AS bound_raw,
				TO_DATE(
					substring(pg_get_expr(c.relpartbound, c.oid) FROM 'FROM \(''(\d{4}-\d{2}-\d{2})'),
					'YYYY-MM-DD'
				) AS extracted_date,
				(CURRENT_DATE - TO_DATE(
					substring(pg_get_expr(c.relpartbound, c.oid) FROM 'FROM \(''(\d{4}-\d{2}-\d{2})'),
					'YYYY-MM-DD'
				))::INT AS age
			FROM partition_tree pt
			JOIN pg_class c ON c.relname = pt.table_name
			WHERE NOT EXISTS (
				SELECT 1 FROM pg_partitioned_table pp WHERE pp.partrelid = c.oid
			)
			AND pt.level > 0
			AND substring(pg_get_expr(c.relpartbound, c.oid) FROM 'FROM \(''(\d{4}-\d{2}-\d{2})') IS NOT NULL
			AND pt.table_name LIKE '%` + recentDate.Format("2006_01_02") + `%'
			ORDER BY pt.table_name
		`).Scan(&recentAfterCleanup).Error
		require.NoError(t, err)

		t.Logf("Recent partitions after cleanup (still attached):")
		if len(recentAfterCleanup) == 0 {
			t.Log("  (none found - all were detached!)")
		} else {
			for _, d := range recentAfterCleanup {
				t.Logf("  %s: bound=%q, extracted_date=%v, age=%v",
					d.TableName, d.BoundRaw, d.ExtractedDate, d.Age)
			}
		}

		// Check if recent partitions exist as detached tables
		var detachedRecentTables []string
		err = dbc.DB.Raw(`
			SELECT tablename
			FROM pg_tables
			WHERE schemaname = 'public'
			  AND tablename LIKE '%` + recentDate.Format("2006_01_02") + `%'
			  AND tablename NOT IN (
				SELECT child.relname
				FROM pg_inherits
				JOIN pg_class child ON pg_inherits.inhrelid = child.oid
			  )
		`).Scan(&detachedRecentTables).Error
		require.NoError(t, err)
		if len(detachedRecentTables) > 0 {
			t.Logf("Recent partitions that exist as detached tables:")
			for _, tbl := range detachedRecentTables {
				t.Logf("  %s", tbl)
			}
		}

		// Verify recent partitions are still attached (recursive walk for nested partitions)
		var recentPartitionExists bool
		err = dbc.DB.Raw(`
			WITH RECURSIVE partition_tree AS (
				SELECT c.oid, c.relname AS table_name
				FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE c.relname IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
				  AND n.nspname = 'public'
				UNION ALL
				SELECT child.oid, child.relname
				FROM partition_tree pt
				JOIN pg_class parent ON parent.relname = pt.table_name
				JOIN pg_inherits i ON i.inhparent = parent.oid
				JOIN pg_class child ON child.oid = i.inhrelid
			)
			SELECT EXISTS(
				SELECT 1 FROM partition_tree
				WHERE table_name LIKE '%` + recentDate.Format("2006_01_02") + `%'
			)
		`).Scan(&recentPartitionExists).Error
		require.NoError(t, err)
		assert.True(t, recentPartitionExists, "recent partitions should still be attached")
	})

	t.Run("dry run mode does not modify database", func(t *testing.T) {
		releases := []string{"4.17"}
		oldDate := time.Now().AddDate(0, 0, -105)

		// Create partitions (may already exist from earlier subtests, that's OK)
		count, err := dbc.EnsurePartitions(releases, oldDate, oldDate.AddDate(0, 0, 1), false)
		require.NoError(t, err)
		require.GreaterOrEqual(t, count, 0, "EnsurePartitions should succeed even if partitions already exist")

		// Get partition count before dry run
		var partitionCountBefore int64
		err = dbc.DB.Raw(`
			SELECT COUNT(*)
			FROM pg_inherits
			JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
			WHERE parent.relname IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
		`).Scan(&partitionCountBefore).Error
		require.NoError(t, err)

		// Run DetachOldPartitions in dry run mode
		detachedCount, err := dbc.DetachOldPartitions(100, true)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, detachedCount, 0, "dry run should return count without modifying")

		// Verify partition count unchanged
		var partitionCountAfter int64
		err = dbc.DB.Raw(`
			SELECT COUNT(*)
			FROM pg_inherits
			JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
			WHERE parent.relname IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
		`).Scan(&partitionCountAfter).Error
		require.NoError(t, err)

		assert.Equal(t, partitionCountBefore, partitionCountAfter,
			"dry run should not modify partition count")
	})
}
