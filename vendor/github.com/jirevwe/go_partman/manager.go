package partman

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/guregu/null.v4"

	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

// todo(raymond): add metrics

// Manager Partition manager
type Manager struct {
	db         *sqlx.DB
	logger     Logger
	config     *Config
	clock      Clock
	hook       Hook
	partitions map[tableName]Partition
	mu         *sync.RWMutex
	wg         *sync.WaitGroup // For testing synchronization
	stop       chan struct{}   // For graceful shutdown
}

func NewManager(options ...Option) (*Manager, error) {
	m := &Manager{
		mu:         &sync.RWMutex{},
		wg:         &sync.WaitGroup{},
		stop:       make(chan struct{}),
		partitions: make(map[tableName]Partition),
	}

	for _, opt := range options {
		err := opt(m)
		if err != nil {
			return nil, err
		}
	}

	if m.db == nil {
		return nil, ErrDbDriverMustNotBeNil
	}

	if m.logger == nil {
		return nil, ErrLoggerMustNotBeNil
	}

	if m.config == nil {
		return nil, ErrConfigMustNotBeNil
	}

	if m.clock == nil {
		return nil, ErrClockMustNotBeNil
	}

	if err := m.runMigrations(context.Background()); err != nil {
		return nil, err
	}

	if err := m.initialize(context.Background(), m.config); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) GetConfig() Config {
	return *m.config
}

// runMigrations runs all the migrations on the management partitions while keeping them backwards compatible
func (m *Manager) runMigrations(ctx context.Context) error {
	migrations := []string{
		createPartmanSchema,
		createParentsTable,
		createTenantsTable,
		createPartitionsTable,
		createValidateTenantFunction,
		createTriggerOnPartitionInsert,
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	for _, migration := range migrations {
		if _, innerErr := tx.ExecContext(ctx, migration); innerErr != nil {
			return fmt.Errorf("failed to run migration: %s, with error %w", migration, innerErr)
		}
	}

	err = tx.Commit()
	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			m.logger.Error("failed to rollback transaction", "error", rollbackErr)
		}
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (m *Manager) initialize(ctx context.Context, config *Config) error {
	// Handle new ParentTables API
	for _, table := range config.Tables {
		m.logger.Info("creating parent table", "table", table.Name)
		id, err := m.CreateParentTable(ctx, table)
		if err != nil {
			return fmt.Errorf("failed to create parent table %s: %w", table.Name, err)
		}

		table.Id = id

		// Import existing partitions for this table
		err = m.importExistingPartitions(ctx, table)
		if err != nil {
			return fmt.Errorf("failed to import existing partitions for table %s: %w", table.Name, err)
		}
	}

	return nil
}

// CreateFuturePartitions creates partitions for all parent tables and pegs all timestamps to UTC
func (m *Manager) CreateFuturePartitions(ctx context.Context, tc Table) error {
	// Determine start time for new partitions
	today := m.clock.Now().UTC()

	// get the tenants for this table
	var tenants []struct {
		ParentTableId string `db:"parent_table_id"`
		TenantId      string `db:"tenant_id"`
	}
	err := m.db.SelectContext(ctx, &tenants, getTenantsQuery, tc.Name, tc.Schema)
	if err != nil {
		return fmt.Errorf("failed to fetch tenants: %w", err)
	}

	for _, te := range tenants {
		// for each tenant, create the future partitions
		for i := uint(0); i < tc.PartitionCount; i++ {
			bounds := Bounds{
				From: today.Add(time.Duration(i) * tc.PartitionInterval).UTC(),
				To:   today.Add(time.Duration(i+1) * tc.PartitionInterval).UTC(),
			}

			// Check if partition already exists
			partitionName := m.generatePartitionName(Tenant{
				TableName:   tc.Name,
				TableSchema: tc.Schema,
				TenantId:    te.TenantId,
			}, bounds)
			exists, innerErr := m.partitionExists(ctx, partitionName, tc.Schema)
			if innerErr != nil {
				return fmt.Errorf("failed to check if partition exists: %w", innerErr)
			}

			if exists {
				m.logger.Info("partition already exists",
					"table", tc.Name,
					"tenant", "",
					"partition", partitionName,
					"from", bounds.From,
					"to", bounds.To)
				continue
			}

			tempTenant := Tenant{
				TableName:   tc.Name,
				TableSchema: tc.Schema,
				TenantId:    te.TenantId,
			}
			// Create the partition
			innerErr = m.createPartition(ctx, tc, tempTenant, bounds)
			if innerErr != nil {
				return fmt.Errorf("failed to create future partition: %w", innerErr)
			}

			m.logger.Info("created future partition",
				"table", tc.Name,
				"tenant", "",
				"partition", partitionName,
				"from", bounds.From,
				"to", bounds.To)
		}
	}

	return nil
}

// partitionExists checks if a partition table already exists
func (m *Manager) partitionExists(ctx context.Context, partitionName, partitionSchemaName string) (bool, error) {
	var exists bool
	err := m.db.QueryRowContext(ctx, getPartitionExists, partitionSchemaName, partitionName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check partition existence: %w", err)
	}

	return exists, nil
}

func (m *Manager) DropOldPartitions(ctx context.Context) error {
	// Get all managed tables and their retention periods
	type managedTable struct {
		TableName       string       `db:"table_name"`
		SchemaName      string       `db:"schema_name"`
		TenantId        string       `db:"tenant_id"`
		RetentionPeriod TimeDuration `db:"retention_period"`
	}

	var tables []managedTable
	if err := m.db.SelectContext(ctx, &tables, getManagedTablesRetentionPeriods); err != nil {
		return fmt.Errorf("failed to fetch managed tables: %w", err)
	}

	for _, table := range tables {
		// Find partitions older than the retention period
		cutoffTime := m.clock.Now().Add(time.Duration(-table.RetentionPeriod))
		m.logger.Info("dropping old partitions",
			"cutoff_time", cutoffTime,
			"table", table.TableName)
		pattern := fmt.Sprintf("%s_%%", table.TableName)
		if len(table.TenantId) > 0 {
			pattern = fmt.Sprintf("%s_%s_%%", table.TableName, table.TenantId)
		}

		var partitions []string
		if err := m.db.SelectContext(ctx, &partitions, partitionsQuery, table.SchemaName, pattern); err != nil {
			return fmt.Errorf("failed to fetch partitions for table %s: %w", table.TableName, err)
		}

		for _, partition := range partitions {
			// Extract date from partition name
			datePart, err := extractDateFromString(partition)
			if err != nil {
				return err
			}

			partitionDate, err := time.Parse(DateNoHyphens, datePart)
			if err != nil {
				m.logger.Error("failed to parse partition date",
					"partition", partition,
					"error", err)
				continue
			}

			// Check if the partition is older than the retention period
			if partitionDate.Before(cutoffTime) {
				if m.hook != nil {
					// run any pre-drop hooks (backup data, upload to object storage)
					// todo(raymond): pass a context with a deadline to this func
					if err = m.hook(ctx, partition); err != nil {
						m.logger.Error("failed to run pre-drop hooks",
							"partition", partition,
							"error", err)
						continue
					}
				}

				m.logger.Info("no hook func was specified",
					"table", table.TableName,
					"partition", partition,
					"date", partitionDate)

				// Drop the partition
				if _, err = m.db.ExecContext(ctx, fmt.Sprintf(dropPartition, table.SchemaName, partition)); err != nil {
					m.logger.Error("failed to drop partition",
						"partition", partition,
						"error", err)
					continue
				}

				m.logger.Info("dropped old partition",
					"table", table.TableName,
					"partition", partition,
					"date", partitionDate)
			}
		}
	}

	return nil
}

// createPartition creates a partition for a table
func (m *Manager) createPartition(ctx context.Context, tc Table, te Tenant, bounds Bounds) error {
	// Generate a partition name based on bounds
	partitionName := m.generatePartitionName(te, bounds)

	// Create SQL for partition
	pQuery, err := m.generatePartitionSQL(partitionName, tc, te, bounds)
	if err != nil {
		return err
	}

	m.logger.Info(pQuery)

	// Execute partition creation
	_, err = m.db.ExecContext(ctx, pQuery)
	if err != nil {
		return err
	}

	return nil
}

// Maintain defines a regularly run maintenance routine
func (m *Manager) Maintain(ctx context.Context) error {
	// fetch all tables and run maintenance tasks
	tables, err := m.GetParentTables(ctx)
	if err != nil {
		return fmt.Errorf("failed to get parent tables: %w", err)
	}

	// Drop old partitions if needed
	if dropErr := m.DropOldPartitions(ctx); dropErr != nil {
		return fmt.Errorf("failed to drop old partitions: %w", dropErr)
	}

	for _, table := range tables {
		// Check for necessary future partitions
		if innerErr := m.CreateFuturePartitions(ctx, table); innerErr != nil {
			return fmt.Errorf("failed to create future partitions: %w", innerErr)
		}
	}

	return nil
}

// generatePartitionSQL generates the name of the partition table
func (m *Manager) generatePartitionSQL(name string, tc Table, te Tenant, b Bounds) (string, error) {
	switch tc.PartitionType {
	case "range":
		return m.generateRangePartitionSQL(name, te, b), nil
	case "list", "hash":
		return "", fmt.Errorf("list and hash partitions are not implemented yet %q", tc.PartitionType)
	default:
		return "", fmt.Errorf("unsupported partition type %q", tc.PartitionType)
	}
}

func (m *Manager) generateRangePartitionSQL(name string, tc Tenant, b Bounds) string {
	if len(tc.TenantId) > 0 {
		return fmt.Sprintf(generatePartitionWithTenantIdQuery,
			tc.TableSchema, name,
			tc.TableSchema, tc.TableName,
			tc.TenantId, b.From.UTC().Format(time.DateOnly),
			tc.TenantId, b.To.UTC().Format(time.DateOnly))
	}
	return fmt.Sprintf(generatePartitionQuery,
		tc.TableSchema, name,
		tc.TableSchema, tc.TableName,
		b.From.UTC().Format(time.DateOnly),
		b.To.UTC().Format(time.DateOnly))
}

func (m *Manager) checkTableColumnsExist(ctx context.Context, tc Table, tenantId string) error {
	if len(tc.TenantIdColumn) > 0 && len(tenantId) > 0 {
		var exists bool
		err := m.db.QueryRowxContext(ctx, checkColumnExists, tc.Schema, tc.Name, tc.TenantIdColumn).Scan(&exists)
		if err != nil {
			return err
		}

		if !exists {
			return fmt.Errorf("table %s does not have a tenant id column", tc.Name)
		}
	}

	var exists bool
	err := m.db.QueryRowxContext(ctx, checkColumnExists, tc.Schema, tc.Name, tc.PartitionBy).Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("table %s does not have a timestamp column named %s", tc.Name, tc.PartitionBy)
	}

	return nil
}

// Update partition name and SQL formatting to use UTC
func (m *Manager) generatePartitionName(tc Tenant, b Bounds) string {
	datePart := b.From.UTC().Format(DateNoHyphens)

	if len(tc.TenantId) > 0 {
		return fmt.Sprintf("%s_%s_%s", tc.TableName, tc.TenantId, datePart)
	}
	return fmt.Sprintf("%s_%s", tc.TableName, datePart)
}

func extractDateFromString(input string) (string, error) {
	// Regular expression to match exactly 8 digits at the end of the string
	re, err := regexp.Compile(`(\d{8})$`)
	if err != nil {
		return "", err
	}

	// Find the match
	matches := re.FindStringSubmatch(input)

	// If a match is found, return it
	if len(matches) > 1 {
		return matches[1], nil
	}

	// Return empty string if no match
	return "", nil
}

// Start begins the maintenance routine
func (m *Manager) Start(ctx context.Context) error {
	if m.config.SampleRate <= 0 {
		if err := m.Maintain(ctx); err != nil {
			m.logger.Error("an error occurred while running maintenance", "error", err)
		}
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.config.SampleRate)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stop:
				return
			case <-ticker.C:
				if err := m.Maintain(ctx); err != nil {
					m.logger.Error("an error occurred while running maintenance", "error", err)
				}
			}
		}
	}()
	return nil
}

// Stop gracefully stops the maintenance routine; used for testing
func (m *Manager) Stop() {
	close(m.stop)
	m.wg.Wait()
}

// generateTableKey creates a unique key for a table based on its name and tenant ID
func generateTableKey(tableName, tenantID string) string {
	if tenantID != "" {
		return fmt.Sprintf("%s_%s", tableName, tenantID)
	}
	return tableName
}

// importExistingPartitions scans the database for existing partitions and adds them to the partition management table
func (m *Manager) importExistingPartitions(ctx context.Context, tc Table) error {
	errString := make([]string, 0)

	// Query to get all tables that look like partitions but aren't yet managed
	type unManagedPartition struct {
		TenantFrom    null.String `db:"tenant_from"`
		TenantTo      null.String `db:"tenant_to"`
		TimestampFrom string      `db:"timestamp_from"`
		TimestampTo   string      `db:"timestamp_to"`
		PartitionName string      `db:"partition_name"`
		PartitionExpr string      `db:"partition_expression"`
		ParentSchema  string      `db:"parent_schema"`
		ParentTable   string      `db:"parent_table"`
	}

	var unManagedPartitions []unManagedPartition
	if err := m.db.SelectContext(ctx, &unManagedPartitions, findUnmanagedPartitionsQuery, tc.Schema, tc.Name); err != nil {
		return fmt.Errorf("failed to query unmanaged partitions: %w", err)
	}

	// Process unmanaged partitions
	for _, p := range unManagedPartitions {
		// Create tenant from imported partition if tenant ID exists
		if len(p.TenantFrom.String) > 0 {
			tenant := Tenant{
				TableName:   tc.Name,
				TableSchema: tc.Schema,
				TenantId:    p.TenantFrom.String,
			}

			m.logger.Info("creating tenant from imported partition", "table_id", tc.Id)

			// Register the tenant
			_, err := m.db.ExecContext(ctx, insertTenantSQL, tenant.TenantId, tc.Id)
			if err != nil {
				m.logger.Error("failed to register tenant from imported partition",
					"partition", p.PartitionName,
					"table", tenant.TableName,
					"tenant", tenant.TenantId,
					"error", err)
				errString = append(errString, err.Error())
				continue
			}

			m.logger.Info("registered tenant from imported partition",
				"table", tenant.TableName,
				"schema", tenant.TableSchema,
				"tenant", tenant.TenantId)
		}

		// check to see if the date part exists
		datePart, err := extractDateFromString(p.PartitionName)
		if err != nil {
			errString = append(errString, err.Error())
			continue
		}

		_, err = time.Parse(DateNoHyphens, datePart)
		if err != nil {
			errString = append(errString, err.Error())
			continue
		}

		parts := strings.Split(p.PartitionName, "_")
		if len(parts) < 2 {
			errString = append(errString, fmt.Sprintf("invalid partition name: %s", p.PartitionName))
			continue
		}

		from, err := time.Parse(time.RFC3339, p.TimestampFrom)
		if err != nil {
			errString = append(errString, fmt.Sprintf("failed to parse from timestamp: %v", err))
			continue
		}

		to, err := time.Parse(time.RFC3339, p.TimestampTo)
		if err != nil {
			errString = append(errString, fmt.Sprintf("failed to parse to timestamp: %v", err))
			continue
		}

		partition := Partition{
			Name:        p.PartitionName,
			Bounds:      Bounds{From: from, To: to},
			TenantId:    p.TenantFrom.String,
			ParentTable: tc,
		}

		err = m.checkTableColumnsExist(ctx, tc, p.TenantFrom.String)
		if err != nil {
			errString = append(errString, err.Error())
			continue
		}

		mTable := partition.toManagedTable()

		// Insert into partition management table
		res, err := m.db.ExecContext(ctx, upsertSQL,
			ulid.Make().String(),
			tc.Id,
			mTable.TenantID,
			mTable.PartitionBy,
			mTable.PartitionType,
			mTable.PartitionBoundsFrom,
			mTable.PartitionBoundsTo,
		)
		if err != nil {
			m.logger.Error("failed to insert management entry",
				"partition", p.PartitionName,
				"table", p.ParentTable,
				"tenant", p.TenantFrom,
				"error", err)
			errString = append(errString, err.Error())
			continue
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			m.logger.Error("failed to get rows affected ",
				"partition", p.PartitionName,
				"table", p.ParentTable,
				"tenant", p.TenantFrom,
				"error", err)
			errString = append(errString, err.Error())
			continue
		}

		if rowsAffected > 0 {
			m.logger.Info("imported existing partitioned table ",
				"partition", p.PartitionName,
				"table ", p.ParentTable,
				"tenant ", p.TenantFrom)

			// Add to our map of unique tables
			key := buildTableName(mTable.SchemaName, mTable.TableName, mTable.TenantID)
			m.addToPartitionMap(key, partition)
		}
	}

	if len(errString) > 0 {
		return errors.New(strings.Join(errString, "; "))
	}
	return nil
}

func (m *Manager) GetManagedTables(ctx context.Context) ([]uiManagedTableInfo, error) {
	var tables []uiManagedTableInfo
	err := m.db.SelectContext(ctx, &tables, getManagedTablesListQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get managed tables: %w", err)
	}
	return tables, nil
}

func (m *Manager) GetPartitions(ctx context.Context, schema, tableName string, limit, offset int) ([]uiPartitionInfo, error) {
	pattern := fmt.Sprintf("%s_%%", tableName)
	var partitions []uiPartitionInfo
	err := m.db.SelectContext(ctx, &partitions, getPartitionDetailsQuery, schema, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get partitions: %w", err)
	}
	return partitions, nil
}

func (m *Manager) GetParentTableInfo(ctx context.Context, schema, tableName string) (*uiParentTableInfo, error) {
	var info uiParentTableInfo
	err := m.db.GetContext(ctx, &info, getParentTableInfoQuery, schema, tableName)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// CreateParentTable registers a parent table for partitioning
func (m *Manager) CreateParentTable(ctx context.Context, table Table) (string, error) {
	if err := table.Validate(); err != nil {
		return "", fmt.Errorf("invalid parent table configuration: %w", err)
	}

	// Check if table columns exist
	if err := m.checkTableColumnsExist(ctx, table, ""); err != nil {
		return "", fmt.Errorf("table validation failed: %w", err)
	}

	parentTableId := struct {
		Id string `db:"id"`
	}{}
	// Insert or update parent table configuration
	err := m.db.QueryRowxContext(ctx, upsertParentTableSQL,
		ulid.Make().String(),
		table.Name,
		table.Schema,
		table.TenantIdColumn,
		table.PartitionBy,
		table.PartitionType,
		table.PartitionInterval.String(),
		table.PartitionCount,
		table.RetentionPeriod.String(),
	).StructScan(&parentTableId)
	if err != nil {
		return "", fmt.Errorf("failed to upsert parent table config for %s: %w", table.Name, err)
	}

	m.logger.Info("created parent table", "table", table.Name, "schema", table.Schema)

	return parentTableId.Id, nil
}

// RegisterTenant registers a tenant for an existing parent table
func (m *Manager) RegisterTenant(ctx context.Context, tenant Tenant) (*TenantRegistrationResult, error) {
	if err := tenant.Validate(); err != nil {
		return nil, fmt.Errorf("invalid tenant configuration: %w", err)
	}

	result := &TenantRegistrationResult{
		TenantId:                   tenant.TenantId,
		TableName:                  tenant.TableName,
		TableSchema:                tenant.TableSchema,
		PartitionsCreated:          0,
		ExistingPartitionsImported: 0,
		Errors:                     []error{},
	}

	// Get parent table configuration
	var parentTableData struct {
		Id                string `db:"id"`
		TableName         string `db:"table_name"`
		SchemaName        string `db:"schema_name"`
		TenantColumn      string `db:"tenant_column"`
		PartitionBy       string `db:"partition_by"`
		PartitionType     string `db:"partition_type"`
		PartitionInterval string `db:"partition_interval"`
		PartitionCount    uint   `db:"partition_count"`
		RetentionPeriod   string `db:"retention_period"`
	}

	err := m.db.GetContext(ctx, &parentTableData, getParentTableQuery, tenant.TableName, tenant.TableSchema)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("parent table not found: %w", err))
		return result, nil
	}

	// Convert to Table
	interval, err := time.ParseDuration(parentTableData.PartitionInterval)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to parse partition interval: %w", err))
		return result, nil
	}

	retention, err := time.ParseDuration(parentTableData.RetentionPeriod)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to parse retention period: %w", err))
		return result, nil
	}

	parentTable := Table{
		Id:                parentTableData.Id,
		Name:              parentTableData.TableName,
		Schema:            parentTableData.SchemaName,
		TenantIdColumn:    parentTableData.TenantColumn,
		PartitionBy:       parentTableData.PartitionBy,
		PartitionType:     PartitionerType(parentTableData.PartitionType),
		PartitionInterval: interval,
		PartitionCount:    parentTableData.PartitionCount,
		RetentionPeriod:   retention,
	}

	// Insert tenant
	_, err = m.db.ExecContext(ctx, insertTenantSQL,
		tenant.TenantId,
		parentTable.Id,
	)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to insert tenant: %w", err))
		return result, nil
	}

	// Create future partitions
	if err = m.CreateFuturePartitions(ctx, parentTable); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to create future partitions: %w", err))
	} else {
		result.PartitionsCreated = int(parentTable.PartitionCount)
	}

	// Import existing partitions
	if err = m.importExistingPartitions(ctx, parentTable); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to import existing partitions: %w", err))
	} else {
		// Count existing partitions (this is a rough estimate)
		result.ExistingPartitionsImported = 1 // We'll improve this later
	}

	m.logger.Info("registered tenant", "tenant", tenant.TenantId, "table", tenant.TableName)
	return result, nil
}

// RegisterTenants registers multiple tenants for an existing parent table (new API)
func (m *Manager) RegisterTenants(ctx context.Context, tenants ...Tenant) ([]TenantRegistrationResult, error) {
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	results := make([]TenantRegistrationResult, 0, len(tenants))

	for _, tenant := range tenants {
		// TODO: pass the db transaction via the context
		result, err := m.RegisterTenant(ctx, tenant)
		if err != nil {
			return results, err
		}
		results = append(results, *result)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return results, nil
}

// GetParentTables returns all registered parent tables
func (m *Manager) GetParentTables(ctx context.Context) ([]Table, error) {
	var parentTables []struct {
		TableName         string `db:"table_name"`
		SchemaName        string `db:"schema_name"`
		TenantColumn      string `db:"tenant_column"`
		PartitionBy       string `db:"partition_by"`
		PartitionType     string `db:"partition_type"`
		PartitionInterval string `db:"partition_interval"`
		PartitionCount    uint   `db:"partition_count"`
		RetentionPeriod   string `db:"retention_period"`
	}

	err := m.db.SelectContext(ctx, &parentTables, getParentTablesQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent tables: %w", err)
	}

	result := make([]Table, 0, len(parentTables))
	for _, pt := range parentTables {
		interval, innerErr := time.ParseDuration(pt.PartitionInterval)
		if innerErr != nil {
			m.logger.Error("failed to parse partition interval", "error", innerErr, "table", pt.TableName)
			continue
		}

		retention, innerErr := time.ParseDuration(pt.RetentionPeriod)
		if innerErr != nil {
			m.logger.Error("failed to parse retention period", "error", innerErr, "table", pt.TableName)
			continue
		}

		result = append(result, Table{
			Name:              pt.TableName,
			Schema:            pt.SchemaName,
			TenantIdColumn:    pt.TenantColumn,
			PartitionBy:       pt.PartitionBy,
			PartitionType:     PartitionerType(pt.PartitionType),
			PartitionInterval: interval,
			PartitionCount:    pt.PartitionCount,
			RetentionPeriod:   retention,
		})
	}

	return result, nil
}

// GetTenants returns all tenants for a specific parent table
func (m *Manager) GetTenants(ctx context.Context, parentTableSchema, parentTableName string) ([]Tenant, error) {
	var tenants []struct {
		ParentTableId string `db:"parent_table_id"`
		TenantId      string `db:"tenant_id"`
	}

	err := m.db.SelectContext(ctx, &tenants, getTenantsQuery, parentTableName, parentTableSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tenants: %w", err)
	}

	m.logger.Info("fetched tenants", "table", parentTableName, "schema", parentTableSchema, "count", len(tenants))

	result := make([]Tenant, 0, len(tenants))
	for _, t := range tenants {
		result = append(result, Tenant{
			TableName:   parentTableName,
			TableSchema: parentTableSchema,
			TenantId:    t.TenantId,
		})
	}

	return result, nil
}

func (m *Manager) addToPartitionMap(key tableName, partition Partition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partitions[key] = partition
}

func (m *Manager) removePartitionFromMap(key tableName) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.partitions, key)
}
