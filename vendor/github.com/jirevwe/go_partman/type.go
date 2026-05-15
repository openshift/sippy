package partman

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type tableName string

func buildTableName(schema string, table string, tenantId string) tableName {
	if schema == "" {
		schema = "public"
	}
	if tenantId != "" && len(tenantId) > 0 {
		return tableName(fmt.Sprintf("%s.%s_%s", schema, table, tenantId))
	}
	return tableName(fmt.Sprintf("%s.%s", schema, table))
}

var (
	ErrHookMustNotBeNil     = errors.New("[partition manager] hook must not be nil")
	ErrClockMustNotBeNil    = errors.New("[partition manager] clock must not be nil")
	ErrLoggerMustNotBeNil   = errors.New("[partition manager] logger must not be nil")
	ErrConfigMustNotBeNil   = errors.New("[partition manager] config must not be nil")
	ErrDbDriverMustNotBeNil = errors.New("[partition manager] db driver must not be nil")
)

// Hook a hook func executes any necessary operations before dropping a partition
// Example hooks:
// 1. Export data to cold storage
// 2. Create backup
// 3. Send notifications
// 4. Update metrics
type Hook func(ctx context.Context, partition string) error

type Option func(m *Manager) error

// WithDB function to set the database
func WithDB(db *sqlx.DB) Option {
	return func(m *Manager) error {
		if db == nil {
			return ErrDbDriverMustNotBeNil
		}

		m.db = db
		return nil
	}
}

// WithLogger function to set the logger
func WithLogger(logger Logger) Option {
	return func(m *Manager) error {
		if logger == nil {
			return ErrLoggerMustNotBeNil
		}

		m.logger = logger
		return nil
	}
}

// WithConfig function to set the config
func WithConfig(config *Config) Option {
	return func(m *Manager) error {
		if config == nil {
			return ErrConfigMustNotBeNil
		}

		if err := config.Validate(); err != nil {
			return err
		}

		m.config = config
		return nil
	}
}

// WithClock function to set the clock
func WithClock(clock Clock) Option {
	return func(m *Manager) error {
		if clock == nil {
			return ErrClockMustNotBeNil
		}

		m.clock = clock
		return nil
	}
}

// WithHook function to set the hook
func WithHook(hook Hook) Option {
	return func(m *Manager) error {
		if hook == nil {
			return ErrHookMustNotBeNil
		}

		m.hook = hook
		return nil
	}
}

type TimeDuration time.Duration

func (t *TimeDuration) Scan(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("unsupported value type %T", value)
	}

	td, err := time.ParseDuration(s)
	if err != nil {
		return err
	}

	*t = TimeDuration(td)

	return nil
}

func (t TimeDuration) Value() (driver.Value, error) {
	duration := time.Duration(t)
	if duration == 0 {
		return nil, nil
	}
	return duration.String(), nil
}

type PartitionerType string

const (
	TypeRange PartitionerType = "range"
)

const (
	DateNoHyphens = "20060102"
)

type Partitioner interface {
	// CreateFuturePartitions Create new partitions ahead of time
	CreateFuturePartitions(ctx context.Context, tableConfig Table) error

	// DropOldPartitions Drop old partitions based on retention policy
	DropOldPartitions(ctx context.Context) error

	// Maintain defines a regularly run maintenance routine
	Maintain(ctx context.Context) error

	// AddManagedTable adds a new managed table to the partition manager
	AddManagedTable(tc Table) error

	// importExistingPartitions scans the database for existing partitions and adds them to the partition management table
	importExistingPartitions(ctx context.Context, tc Table) error

	// CreateParentTable registers a parent table for partitioning (new API)
	CreateParentTable(ctx context.Context, parentTable Table) error

	// RegisterTenant registers a tenant for an existing parent table (new API)
	RegisterTenant(ctx context.Context, tenant Tenant) (*TenantRegistrationResult, error)

	// RegisterTenants registers multiple tenants for an existing parent table (new API)
	RegisterTenants(ctx context.Context, tenants []Tenant) ([]TenantRegistrationResult, error)

	// GetParentTables returns all registered parent tables
	GetParentTables(ctx context.Context) ([]Table, error)

	// GetTenants returns all tenants for a specific parent table
	GetTenants(ctx context.Context, parentTableName, parentTableSchema string) ([]Tenant, error)
}

type Bounds struct {
	From, To time.Time
}

type D struct {
	Key   string
	Value string
}

type Partition struct {
	// Name the name of the partition within the partitioned table.
	Name string

	// ParentTable the metadata and configuration of the parent table for the partition.
	ParentTable Table

	// Bounds the range of time covered by the partition, defined by start (From) and end (To) timestamps.
	Bounds Bounds

	// TenantId the tenant ID column value (e.g., 01J2V010NV1259CYWQEYQC8F35)
	TenantId string
}

func (p *Partition) toManagedTable() managedTable {
	return managedTable{
		TableName:           p.Name,
		SchemaName:          p.ParentTable.Schema,
		TenantID:            p.TenantId,
		TenantColumn:        p.ParentTable.TenantIdColumn,
		PartitionBy:         p.ParentTable.PartitionBy,
		PartitionType:       string(p.ParentTable.PartitionType),
		PartitionBoundsFrom: p.Bounds.From,
		PartitionBoundsTo:   p.Bounds.To,
	}
}

type Table struct {
	Id string

	// Name of the partitioned table
	Name string

	// Schema of the partitioned table
	Schema string

	// TenantIdColumn Tenant ID column to partition by (e.g., tenant_id)
	TenantIdColumn string

	// PartitionBy Timestamp column to partition by (e.g., created_at)
	PartitionBy string

	// PartitionType Postgres partition type
	PartitionType PartitionerType // "range", "list", or "hash"

	// PartitionInterval For range partitions (e.g., "1 month", "1 day")
	PartitionInterval time.Duration

	// PartitionCount is the number of partitions a table will have; defaults to 10
	PartitionCount uint

	// RetentionPeriod is how long after which partitions will be dropped (e.g., "1 month", "1 day")
	RetentionPeriod time.Duration
}

type Config struct {
	// SampleRate is how often the internal ticker runs
	SampleRate time.Duration

	// Tables holds parent table configurations
	Tables []Table
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate legacy Tables API
	for i, table := range c.Tables {
		if err := table.Validate(); err != nil {
			return fmt.Errorf("table[%d]: %w", i, err)
		}
	}

	return nil
}

// Validate checks if the table configuration is valid
func (tc *Table) Validate() error {
	if tc.Name == "" {
		return errors.New("name cannot be empty")
	}

	if tc.Schema == "" {
		return errors.New("schema name cannot be empty")
	}

	if tc.RetentionPeriod == 0 {
		return errors.New("retention period must be set")
	}

	// set default value
	if tc.PartitionCount == 0 {
		tc.PartitionCount = 10
	}

	if tc.PartitionType == TypeRange {
		if len(tc.PartitionBy) == 0 {
			return errors.New("partition_by is required for range partitions")
		}

		if tc.PartitionInterval == 0 {
			return errors.New("partition interval must be set for range partitions")
		}
	}

	return nil
}

type StringArray []string

func (a *StringArray) Scan(src interface{}) error {
	if src == nil {
		return nil
	}

	var array []string

	switch v := src.(type) {
	case string:
		array = append(array, v)
	case []string:
		array = v
	}

	*a = array

	return nil
}

type managedTable struct {
	TableName           string    `db:"table_name"`
	SchemaName          string    `db:"schema_name"`
	TenantID            string    `db:"tenant_id"`
	TenantColumn        string    `db:"tenant_column"`
	PartitionBy         string    `db:"partition_by"`
	PartitionType       string    `db:"partition_type"`
	PartitionBoundsFrom time.Time `db:"partition_bounds_from"`
	PartitionBoundsTo   time.Time `db:"partition_bounds_to"`
}

type uiPartitionInfo struct {
	Name       string `json:"name" db:"name"`
	Size       string `json:"size" db:"size"`
	Rows       int64  `json:"rows" db:"rows"`
	Range      string `json:"range" db:"range"`
	Created    string `json:"created" db:"created"`
	SizeBytes  int64  `json:"size_bytes" db:"size_bytes"`
	TotalCount int64  `json:"total_count" db:"total_count"`
}

type uiParentTableInfo struct {
	Name           string `json:"name" db:"name"`
	TotalSize      string `json:"total_size" db:"total_size"`
	TotalRows      int64  `json:"total_rows" db:"total_rows"`
	PartitionCount int    `json:"partition_count" db:"partition_count"`
	TotalSizeBytes int64  `json:"total_size_bytes" db:"total_size_bytes"`
}

type uiManagedTableInfo struct {
	Name   string `json:"name" db:"table_name"`
	Schema string `json:"schema" db:"schema_name"`
}

// Tenant represents a tenant configuration for a specific parent table
type Tenant struct {
	// ParentTableName references the parent table this tenant belongs to
	TableName string

	// ParentTableSchema references the parent table schema
	TableSchema string

	// TenantId Tenant ID column value (e.g., 01J2V010NV1259CYWQEYQC8F35)
	TenantId string
}

// TenantRegistrationResult contains the result of registering a tenant
type TenantRegistrationResult struct {
	// TenantId the tenant ID that was registered
	TenantId string

	// TableName the parent table name
	TableName string

	// TableSchema the parent table schema
	TableSchema string

	// PartitionsCreated number of partitions created for this tenant
	PartitionsCreated int

	// ExistingPartitionsImported number of existing partitions imported
	ExistingPartitionsImported int

	// Errors any errors encountered during registration
	Errors []error
}

// Validate checks if the tenant configuration is valid
func (t *Tenant) Validate() error {
	if t.TableSchema == "" {
		return errors.New("table schema cannot be empty")
	}

	if t.TableName == "" {
		return errors.New("table name cannot be empty")
	}

	if t.TenantId == "" {
		return errors.New("tenant ID cannot be empty")
	}

	return nil
}
