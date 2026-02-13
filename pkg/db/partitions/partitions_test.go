package partitions

import (
	"testing"
	"time"
)

func TestIsValidTestAnalysisPartitionName(t *testing.T) {
	tests := []struct {
		name      string
		partition string
		want      bool
	}{
		{
			name:      "valid partition name",
			partition: "test_analysis_by_job_by_dates_2024_10_29",
			want:      true,
		},
		{
			name:      "valid partition name 2026",
			partition: "test_analysis_by_job_by_dates_2026_01_15",
			want:      true,
		},
		{
			name:      "invalid - too short",
			partition: "test_analysis_by_job_by_dates",
			want:      false,
		},
		{
			name:      "invalid - wrong prefix",
			partition: "wrong_analysis_by_job_by_dates_2024_10_29",
			want:      false,
		},
		{
			name:      "invalid - wrong date format",
			partition: "test_analysis_by_job_by_dates_2024_13_40",
			want:      false,
		},
		{
			name:      "invalid - SQL injection attempt",
			partition: "test_analysis_by_job_by_dates_2024_10_29; DROP TABLE prow_jobs;",
			want:      false,
		},
		{
			name:      "invalid - missing date",
			partition: "test_analysis_by_job_by_dates_",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidPartitionName("test_analysis_by_job_by_dates", tt.partition)
			if got != tt.want {
				t.Errorf("isValidTestAnalysisPartitionName(%q) = %v, want %v", tt.partition, got, tt.want)
			}
		})
	}
}

func TestPartitionInfo(t *testing.T) {
	// Test that PartitionInfo struct can be instantiated
	partition := PartitionInfo{
		TableName:     "test_analysis_by_job_by_dates_2024_10_29",
		SchemaName:    "public",
		PartitionDate: time.Date(2024, 10, 29, 0, 0, 0, 0, time.UTC),
		Age:           100,
		SizeBytes:     1073741824, // 1 GB
		SizePretty:    "1 GB",
		RowEstimate:   1000000,
	}

	if partition.TableName != "test_analysis_by_job_by_dates_2024_10_29" {
		t.Errorf("unexpected table name: %s", partition.TableName)
	}
}

func TestRetentionSummary(t *testing.T) {
	// Test that RetentionSummary struct can be instantiated
	summary := RetentionSummary{
		RetentionDays:      180,
		CutoffDate:         time.Now().AddDate(0, 0, -180),
		PartitionsToRemove: 50,
		StorageToReclaim:   53687091200, // ~50 GB
		StoragePretty:      "50 GB",
		OldestPartition:    "test_analysis_by_job_by_dates_2024_10_29",
		NewestPartition:    "test_analysis_by_job_by_dates_2024_12_17",
	}

	if summary.RetentionDays != 180 {
		t.Errorf("unexpected retention days: %d", summary.RetentionDays)
	}

	if summary.PartitionsToRemove != 50 {
		t.Errorf("unexpected partitions to remove: %d", summary.PartitionsToRemove)
	}
}
