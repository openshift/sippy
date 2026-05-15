package partman

var createPartmanSchema = `CREATE SCHEMA IF NOT EXISTS partman;`

var createParentsTable = `
CREATE TABLE IF NOT EXISTS partman.parent_tables (
    id VARCHAR PRIMARY KEY,
    schema_name VARCHAR NOT NULL,
    table_name VARCHAR NOT NULL,
    tenant_column VARCHAR,
    partition_by VARCHAR NOT NULL,
    partition_type VARCHAR NOT NULL,
    partition_interval VARCHAR NOT NULL,
    partition_count INT NOT NULL DEFAULT 10,
    retention_period VARCHAR NOT NULL,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(schema_name, table_name)
);`

var createTenantsTable = `
CREATE TABLE IF NOT EXISTS partman.tenants (
    id VARCHAR NOT NULL,
    parent_table_id VARCHAR NOT NULL,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (parent_table_id, id),
    FOREIGN KEY (parent_table_id) 
        REFERENCES partman.parent_tables(id) ON DELETE CASCADE,
    UNIQUE(parent_table_id, id)
);`

var createPartitionsTable = `
CREATE TABLE IF NOT EXISTS partman.partitions (
    id VARCHAR PRIMARY KEY,
    parent_table_id VARCHAR NOT NULL,
    tenant_id VARCHAR,
    partition_by VARCHAR NOT NULL,
    partition_type VARCHAR NOT NULL,
    partition_bounds_from TIMESTAMPTZ NOT NULL,
    partition_bounds_to TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (parent_table_id) 
        REFERENCES partman.parent_tables(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_table_id, tenant_id) 
        REFERENCES partman.tenants(parent_table_id, id) ON DELETE CASCADE,
    UNIQUE(parent_table_id, tenant_id, partition_bounds_from, partition_bounds_to)
);`

var createValidateTenantFunction = `
CREATE OR REPLACE FUNCTION partman.validate_tenant_id() RETURNS TRIGGER AS $$
BEGIN
    -- If tenant_id is provided, ensure it exists for this parent table
    IF NEW.tenant_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM partman.tenants 
            WHERE parent_table_id = NEW.parent_table_id 
            AND id = NEW.tenant_id
        ) THEN
            RAISE EXCEPTION 'Tenant % does not exist for parent table %', 
                NEW.tenant_id, NEW.parent_table_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;`

var createTriggerOnPartitionInsert = `
CREATE OR REPLACE TRIGGER validate_tenant_id_trigger
    BEFORE INSERT OR UPDATE ON partman.partitions
    FOR EACH ROW EXECUTE FUNCTION partman.validate_tenant_id()`

var upsertSQL = `
INSERT INTO partman.partitions (
	id, parent_table_id, tenant_id, partition_by, partition_type,
	partition_bounds_from, partition_bounds_to
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING;`

var getlatestPartition = `
SELECT tablename 
FROM pg_tables 
WHERE schemaname = $1 AND tablename LIKE $2 
ORDER BY tablename DESC 
LIMIT 1;`

// todo(raymond): paginate this query?
var getManagedTablesRetentionPeriods = `
SELECT pt.table_name, pt.schema_name, p.tenant_id, pt.retention_period 
FROM partman.partitions p
join partman.parent_tables pt on pt.id = p.parent_table_id;`

var getPartitionExists = `
SELECT EXISTS (
	SELECT 1 
	FROM pg_tables 
	WHERE schemaname = $1 AND tablename = $2
);`

var partitionsQuery = `
SELECT tablename 
FROM pg_tables
WHERE schemaname = $1 AND tablename ILIKE $2;`

var dropPartition = `DROP TABLE IF EXISTS %s.%s;`

var generatePartitionQuery = `CREATE TABLE IF NOT EXISTS %s.%s PARTITION OF %s.%s FOR VALUES FROM ('%s 00:00:00+00'::timestamptz) TO ('%s 00:00:00+00'::timestamptz);`

var generatePartitionWithTenantIdQuery = `CREATE TABLE IF NOT EXISTS %s.%s PARTITION OF %s.%s FOR VALUES FROM ('%s', '%s 00:00:00+00'::timestamptz) TO ('%s', '%s 00:00:00+00'::timestamptz);`

var checkColumnExists = `
SELECT EXISTS (SELECT 1 
FROM information_schema.columns
WHERE table_schema=$1 AND table_name=$2 AND column_name=$3);`

var getManagedTablesQuery = `
SELECT 
    pt.table_name,
    pt.schema_name,
    p.tenant_id,
    pt.tenant_column,
    pt.partition_by,
    pt.partition_type,
    p.partition_bounds_from,
    p.partition_bounds_to
FROM partman.partitions p 
join partman.parent_tables pt on p.parent_table_id = pt.id;`

var getPartitionDetailsQuery = `
WITH partition_info AS (
    SELECT
        t.tablename as name,
        pg_total_relation_size(quote_ident($1) || '.' || quote_ident(t.tablename)) as size_bytes,
        (SELECT reltuples::bigint FROM pg_class WHERE oid = (quote_ident($1) || '.' || quote_ident(t.tablename))::regclass) as rows,
        p.created_at as created
    FROM pg_tables t
    LEFT JOIN partman.parent_tables pt ON pt.schema_name = t.schemaname AND pt.table_name = $2
    LEFT JOIN partman.partitions p ON p.parent_table_id = pt.id
    WHERE t.schemaname = $1 AND t.tablename LIKE $2 || '_%'
    ORDER BY t.tablename DESC
    LIMIT $3 OFFSET $4
)
SELECT
    name,
    pg_size_pretty(size_bytes) as size,
    rows,
    COALESCE(to_char(created, 'YYYY-MM-DD HH24:MI:SS'), '') as created,
    size_bytes,
    (SELECT COUNT(*) FROM pg_tables WHERE schemaname = $1 AND tablename LIKE $2 || '_%') as total_count
FROM partition_info;`

var getParentTableInfoQuery = `
WITH parent_table_info AS (
    SELECT
        schemaname,
        tablename,
        (SELECT COUNT(*) FROM pg_tables WHERE schemaname = $1 AND tablename LIKE $2 || '_%') as partition_count
    FROM pg_tables
    WHERE schemaname = $1 AND tablename = $2
),
partition_sizes AS (
    SELECT
        schemaname,
        tablename,
        pg_total_relation_size(quote_ident(schemaname) || '.' || quote_ident(tablename)) as size_bytes,
        (SELECT reltuples::bigint FROM pg_class WHERE oid = (quote_ident(schemaname) || '.' || quote_ident(tablename))::regclass) as estimated_rows
    FROM pg_tables
    WHERE schemaname = $1 AND tablename LIKE $2 || '_%'
),
totals AS (
    SELECT
        COALESCE(SUM(size_bytes), 0) as total_size_bytes,
        COALESCE(SUM(estimated_rows), 0) as total_rows
    FROM partition_sizes
)
SELECT
    pti.tablename as name,
    pg_size_pretty(t.total_size_bytes) as total_size,
    t.total_rows as total_rows,
    pti.partition_count as partition_count,
    t.total_size_bytes as total_size_bytes
FROM parent_table_info pti
CROSS JOIN totals t;`

var getManagedTablesListQuery = `
SELECT DISTINCT table_name, schema_name 
FROM partman.parent_tables
ORDER BY table_name;`

var upsertParentTableSQL = `
INSERT INTO partman.parent_tables (
    id, table_name, schema_name, 
	tenant_column, partition_by, 
    partition_type, partition_interval,
	partition_count, retention_period
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (schema_name, table_name)
DO UPDATE SET updated_at = current_timestamp
RETURNING id;`

var insertTenantSQL = `
INSERT INTO partman.tenants (id, parent_table_id) 
VALUES ($1, $2)
ON CONFLICT DO NOTHING;`

var getParentTablesQuery = `
SELECT 
    table_name,
    schema_name,
    tenant_column,
    partition_by,
    partition_type,
    partition_interval,
    partition_count,
    retention_period
FROM partman.parent_tables
ORDER BY table_name;`

var getTenantsQuery = `
SELECT t.id as tenant_id, pt.id as parent_table_id
FROM partman.tenants t
JOIN partman.parent_tables pt ON pt.id = t.parent_table_id 
WHERE pt.table_name = $1 AND pt.schema_name = $2
ORDER BY t.id;`

var getParentTableQuery = `
SELECT 
    id,
    table_name,
    schema_name,
    tenant_column,
    partition_by,
    partition_type,
    partition_interval,
    partition_count,
    retention_period
FROM partman.parent_tables
WHERE table_name = $1 AND schema_name = $2;`

var findUnmanagedPartitionsQuery = `
WITH bounds AS (
SELECT
	nmsp_parent.nspname AS parent_schema,
	parent.relname AS parent_table,
	nmsp_child.nspname AS partition_schema,
	child.relname AS partition_name,
	pg_get_expr(child.relpartbound, child.oid) AS partition_expression
FROM pg_inherits
		 JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
		 JOIN pg_class child ON pg_inherits.inhrelid = child.oid
		 JOIN pg_namespace nmsp_parent ON nmsp_parent.oid = parent.relnamespace
		 JOIN pg_namespace nmsp_child ON nmsp_child.oid = child.relnamespace
WHERE parent.relkind = 'p'  AND nmsp_parent.nspname = $1 and parent.relname = $2
),
	 parsed_values AS (
		 SELECT
			 *,
			 regexp_matches(partition_expression, 'FROM \(([^)]+)\) TO \(([^)]+)\)', 'g') as extracted_values,
			 (regexp_matches(partition_expression, 'FROM \(([^)]+)\)', 'g'))[1] as from_values,
			 (regexp_matches(partition_expression, 'TO \(([^)]+)\)', 'g'))[1] as to_values
		 FROM bounds
	 )
SELECT
	parent_schema,
	parent_table,
	partition_name,
	partition_expression,
	CASE
		WHEN from_values LIKE '%,%' THEN replace(split_part(from_values, ', ', 1), '''', '')
		END as tenant_from,
	(CASE
		WHEN from_values LIKE '%,%' THEN split_part(from_values, ', ', 2)
		ELSE from_values
		END)::TIMESTAMP as timestamp_from,
	CASE
		WHEN to_values LIKE '%,%' THEN replace(split_part(to_values, ', ', 1), '''', '')
		END as tenant_to,
	(CASE
		WHEN to_values LIKE '%,%' THEN split_part(to_values, ', ', 2)
		ELSE to_values
		END)::TIMESTAMP as timestamp_to
FROM parsed_values;
`
