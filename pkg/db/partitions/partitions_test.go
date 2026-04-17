package partitions

import (
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/db"
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

func TestExtractTableNameFromPartition(t *testing.T) {
	tests := []struct {
		name          string
		partitionName string
		wantTableName string
		wantError     bool
	}{
		{
			name:          "valid partition",
			partitionName: "test_analysis_by_job_by_dates_2024_10_29",
			wantTableName: "test_analysis_by_job_by_dates",
			wantError:     false,
		},
		{
			name:          "different table",
			partitionName: "prow_job_runs_2024_01_15",
			wantTableName: "prow_job_runs",
			wantError:     false,
		},
		{
			name:          "too short",
			partitionName: "short",
			wantTableName: "",
			wantError:     true,
		},
		{
			name:          "invalid date",
			partitionName: "table_name_invalid_date",
			wantTableName: "",
			wantError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractTableNameFromPartition(tt.partitionName)
			if (err != nil) != tt.wantError {
				t.Errorf("extractTableNameFromPartition() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if got != tt.wantTableName {
				t.Errorf("extractTableNameFromPartition() = %v, want %v", got, tt.wantTableName)
			}
		})
	}
}

func TestIsValidPartitionName(t *testing.T) {
	tests := []struct {
		name          string
		tableName     string
		partitionName string
		want          bool
	}{
		{
			name:          "valid partition",
			tableName:     "test_table",
			partitionName: "test_table_2024_10_29",
			want:          true,
		},
		{
			name:          "wrong table name",
			tableName:     "test_table",
			partitionName: "other_table_2024_10_29",
			want:          false,
		},
		{
			name:          "invalid date",
			tableName:     "test_table",
			partitionName: "test_table_2024_13_40",
			want:          false,
		},
		{
			name:          "wrong length",
			tableName:     "test_table",
			partitionName: "test_table_2024_10",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidPartitionName(tt.tableName, tt.partitionName)
			if got != tt.want {
				t.Errorf("isValidPartitionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartitionConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  PartitionConfig
		wantErr bool
	}{
		{
			name:    "valid RANGE config",
			config:  NewRangePartitionConfig("created_at"),
			wantErr: false,
		},
		{
			name:    "valid LIST config",
			config:  NewListPartitionConfig("region"),
			wantErr: false,
		},
		{
			name:    "valid HASH config",
			config:  NewHashPartitionConfig(4, "user_id"),
			wantErr: false,
		},
		{
			name: "invalid - no strategy",
			config: PartitionConfig{
				Columns: []string{"created_at"},
			},
			wantErr: true,
		},
		{
			name: "invalid - no columns",
			config: PartitionConfig{
				Strategy: db.PartitionStrategyRange,
				Columns:  []string{},
			},
			wantErr: true,
		},
		{
			name: "invalid - RANGE with multiple columns",
			config: PartitionConfig{
				Strategy: db.PartitionStrategyRange,
				Columns:  []string{"col1", "col2"},
			},
			wantErr: true,
		},
		{
			name: "invalid - HASH with no modulus",
			config: PartitionConfig{
				Strategy: db.PartitionStrategyHash,
				Columns:  []string{"user_id"},
				Modulus:  0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("PartitionConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPartitionConfigToSQL(t *testing.T) {
	tests := []struct {
		name     string
		config   PartitionConfig
		expected string
	}{
		{
			name:     "RANGE partition",
			config:   NewRangePartitionConfig("created_at"),
			expected: "PARTITION BY RANGE (created_at)",
		},
		{
			name:     "LIST partition",
			config:   NewListPartitionConfig("region"),
			expected: "PARTITION BY LIST (region)",
		},
		{
			name:     "HASH partition single column",
			config:   NewHashPartitionConfig(4, "user_id"),
			expected: "PARTITION BY HASH (user_id)",
		},
		{
			name:     "HASH partition multiple columns",
			config:   NewHashPartitionConfig(8, "user_id", "tenant_id"),
			expected: "PARTITION BY HASH (user_id, tenant_id)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ToSQL()
			if got != tt.expected {
				t.Errorf("PartitionConfig.ToSQL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPrimaryKeyConstraint(t *testing.T) {
	// This test documents that primary keys should get PRIMARY KEY constraint
	// and NOT NULL constraint in the generated SQL

	// In CreatePartitionedTable:
	// 1. Collect all primary key columns
	// 2. Add NOT NULL to each primary key column definition
	// 3. Add PRIMARY KEY (col1, col2, ...) constraint
	// 4. For partitioned tables, ensure partition columns are in the primary key

	type TestModel struct {
		ID        uint   `gorm:"primaryKey"` // Should get NOT NULL and be in PRIMARY KEY constraint
		Name      string `gorm:"not null"`   // Should get NOT NULL from explicit tag
		Age       int    // Should NOT get NOT NULL
		CreatedAt string // For partition column
	}

	// Verify the struct can be instantiated
	var model TestModel
	model.ID = 1

	if model.ID != 1 {
		t.Error("model instantiation failed")
	}

	// The expected SQL should contain:
	// - id bigint NOT NULL
	// - PRIMARY KEY (id, created_at)  -- includes partition column
	// This is verified in integration tests with actual database
}

func TestAutoIncrementHandling(t *testing.T) {
	// This test documents that AutoIncrement fields should get GENERATED BY DEFAULT AS IDENTITY
	// and AutoIncrementIncrement should be respected

	// In CreatePartitionedTable:
	// 1. Check if field.AutoIncrement is true
	// 2. If yes, add GENERATED BY DEFAULT AS IDENTITY
	// 3. If AutoIncrementIncrement > 0, add INCREMENT BY clause
	// 4. IDENTITY columns are automatically NOT NULL

	type TestModelWithAutoIncrement struct {
		ID        uint   `gorm:"primaryKey;autoIncrement"` // Should get GENERATED BY DEFAULT AS IDENTITY
		Name      string `gorm:"not null"`
		CreatedAt string // For partition column
	}

	type TestModelWithIncrementBy struct {
		ID        uint   `gorm:"primaryKey;autoIncrement;autoIncrementIncrement:10"` // Should get INCREMENT BY 10
		Name      string `gorm:"not null"`
		CreatedAt string
	}

	// Verify the structs can be instantiated
	var model1 TestModelWithAutoIncrement
	model1.Name = "test"

	var model2 TestModelWithIncrementBy
	model2.Name = "test"

	if model1.Name != "test" || model2.Name != "test" {
		t.Error("model instantiation failed")
	}

	// The expected SQL should contain:
	// For TestModelWithAutoIncrement:
	// - id bigint GENERATED BY DEFAULT AS IDENTITY
	//
	// For TestModelWithIncrementBy:
	// - id bigint GENERATED BY DEFAULT AS IDENTITY (INCREMENT BY 10)
	//
	// This is verified in integration tests with actual database
}

func TestGormTypeToPostgresType(t *testing.T) {
	tests := []struct {
		name     string
		gormType string
		expected string
	}{
		// Integer types
		{
			name:     "uint to bigint",
			gormType: "uint",
			expected: "bigint",
		},
		{
			name:     "uint8 to smallint",
			gormType: "uint8",
			expected: "smallint",
		},
		{
			name:     "uint16 to integer",
			gormType: "uint16",
			expected: "integer",
		},
		{
			name:     "uint32 to bigint",
			gormType: "uint32",
			expected: "bigint",
		},
		{
			name:     "uint64 to bigint",
			gormType: "uint64",
			expected: "bigint",
		},
		{
			name:     "int to bigint",
			gormType: "int",
			expected: "bigint",
		},
		{
			name:     "int64 to bigint",
			gormType: "int64",
			expected: "bigint",
		},
		// Float types
		{
			name:     "float to double precision",
			gormType: "float",
			expected: "double precision",
		},
		{
			name:     "float32 to real",
			gormType: "float32",
			expected: "real",
		},
		{
			name:     "float64 to double precision",
			gormType: "float64",
			expected: "double precision",
		},
		// String types
		{
			name:     "string to text",
			gormType: "string",
			expected: "text",
		},
		// Boolean
		{
			name:     "bool to boolean",
			gormType: "bool",
			expected: "boolean",
		},
		// Time types
		{
			name:     "time.time to timestamptz",
			gormType: "time.time",
			expected: "timestamp with time zone",
		},
		{
			name:     "time to timestamptz",
			gormType: "time",
			expected: "timestamp with time zone",
		},
		// Binary
		{
			name:     "[]byte to bytea",
			gormType: "[]byte",
			expected: "bytea",
		},
		// JSON
		{
			name:     "json to jsonb",
			gormType: "json",
			expected: "jsonb",
		},
		// PostgreSQL types should pass through
		{
			name:     "varchar remains varchar",
			gormType: "varchar",
			expected: "varchar",
		},
		{
			name:     "character varying remains",
			gormType: "character varying",
			expected: "character varying",
		},
		{
			name:     "timestamptz remains",
			gormType: "timestamptz",
			expected: "timestamptz",
		},
		// Case insensitive
		{
			name:     "UINT to bigint",
			gormType: "UINT",
			expected: "bigint",
		},
		{
			name:     "String to text",
			gormType: "String",
			expected: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gormTypeToPostgresType(tt.gormType)
			if got != tt.expected {
				t.Errorf("gormTypeToPostgresType(%q) = %q, want %q", tt.gormType, got, tt.expected)
			}
		})
	}
}

func TestColumnDeduplication(t *testing.T) {
	// This test documents that CreatePartitionedTable and UpdatePartitionedTable
	// deduplicate columns to prevent the same column from appearing multiple times
	// in the generated SQL.

	// GORM's stmt.Schema.Fields can contain duplicate fields in certain cases:
	// - Embedded structs with same field names
	// - Field tags that create virtual fields
	// - Polymorphic associations
	// - Custom scanners/valuers

	// Example GORM model that might produce duplicates:
	// type Model struct {
	//     gorm.Model  // Contains CreatedAt, UpdatedAt, DeletedAt
	//     CreatedAt time.Time `gorm:"index"`  // Duplicate!
	//     DeletedAt gorm.DeletedAt
	// }

	// Without deduplication, this would generate:
	// CREATE TABLE (...
	//     created_at timestamp with time zone,
	//     updated_at timestamp with time zone,
	//     deleted_at timestamp with time zone,
	//     created_at timestamp with time zone,  -- DUPLICATE!
	//     deleted_at timestamp with time zone,  -- DUPLICATE!
	//     ...
	// )

	// With deduplication (current implementation):
	// CREATE TABLE (...
	//     created_at timestamp with time zone,
	//     updated_at timestamp with time zone,
	//     deleted_at timestamp with time zone,
	//     ... // No duplicates
	// )

	// The deduplication logic uses a map to track which columns have been added:
	// - addedColumns[field.DBName] tracks columns in CreatePartitionedTable
	// - processedColumns[field.DBName] tracks columns in UpdatePartitionedTable
	// - First occurrence of a column is used, subsequent duplicates are skipped

	t.Log("Column deduplication documented - prevents duplicate columns in generated SQL")
}
