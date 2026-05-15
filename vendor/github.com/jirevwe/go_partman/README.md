# go_partman

A Go native implementation of PostgreSQL table partitioning management, inspired by [pg_partman](https://github.com/pgpartman/pg_partman). This library helps you automatically manage and maintain partitioned tables in PostgreSQL databases.

> Disclaimers: 
> 1. This library is currently in alpha, hence the public APIs might change.
> 2. This library was built and is currently used to manage retention policies in [Convoy](https://github.com/frain-dev/convoy).
> 3. It is currently behind a feature flag (I'll update this disclaimer once it's GA).
> 4. This is the accompanying [pull request](https://github.com/frain-dev/convoy/pull/2194/files#diff-6c0399450dc8551e4cd42255ec24371c113d5b7771f6c6fdc0387cb0bc3df7f2) in Convoy if you want to see how it is integrated. 

#### Built By
<a href="https://getconvoy.io/?utm_source=go_partman">
<img src="https://getconvoy.io/svg/convoy-logo-full-new.svg" alt="Sponsored by Convoy"></a>

## Features

- Automatic partition creation and management
- Support for time-based range partitioning
- Configurable retention policies
- Automatic cleanup of old partitions
- Pre-creation of future partitions
- Multi-tenant support with tenant-specific partitioning
- Extensible pre-drop hooks for custom cleanup logic

## Installation

```bash
go get github.com/jirevwe/go_partman
```

## Usage

### Basic Setup

```go
package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jirevwe/go_partman"
	"github.com/jmoiron/sqlx"

	"log"
	"log/slog"
	"os"

	"time"
)

func main() {
	pgxCfg, err := pgxpool.ParseConfig("postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), pgxCfg)
	if err != nil {
		log.Fatal(err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	db := sqlx.NewDb(sqlDB, "pgx")

	config := &partman.Config{
		SampleRate: 30 * time.Second,
		SchemaName: "convoy",
	}

	clock := partman.NewRealClock()
	manager, err := partman.NewAndStart(db, config, slog.New(slog.NewTextHandler(os.Stdout, nil)), clock)
	if err != nil {
		log.Fatal(err)
	}

	if err = manager.Start(context.Background()); err != nil {
		log.Fatal(err)
	}

	time.Sleep(30 * time.Second)
}

```

### Multi-tenant Setup

```go
config := partman.Config{
    SchemaName: "public",
    Tables: []partman.TableConfig{
        {
            Name:              "events",
            TenantId:          "tenant1",           // Specify tenant ID
            TenantIdColumn:    "project_id",        // Column name for tenant ID
            PartitionType:     partman.TypeRange,
            PartitionBy:       "created_at",
            PartitionInterval: time.Hour * 24,
            PartitionCount:    10, 
            RetentionPeriod:   time.Hour * 24 * 7,
        },
    },
}
```

### Adding a Managed Table

You can add a new managed table to the partition manager using the `AddManagedTable` method:

```go
newTableConfig := partman.TableConfig{
    Name:              "new_events",
    TenantId:          "tenant1",           // Specify tenant ID
    TenantIdColumn:    "project_id",        // Column name for tenant ID
    PartitionType:     partman.TypeRange,
    PartitionBy:       "created_at",
    PartitionInterval: time.Hour * 24,
    PartitionCount:    10,
    RetentionPeriod:   time.Hour * 24 * 7,
}

// Add the new managed table
if err := manager.AddManagedTable(newTableConfig); err != nil {
    log.Fatal(err)
}
```

### Import Exsiting Partitions

You can add a new managed table to the partition manager using the `AddManagedTable` method:

```go
err = manager.ImportExistingPartitions(context.Background(), partman.Table{
    TenantIdColumn:    "project_id",
    PartitionBy:       "created_at",
    PartitionType:     partman.TypeRange,
    PartitionInterval: time.Hour * 24,
    PartitionCount:    10,
    RetentionPeriod:   time.Hour * 24 * 7,
})
if err != nil {
    log.Fatal(err)
}
```

### Table Requirements

Your table must be created as a partitioned table before using go_partman. Examples:

```sql
-- Single-tenant table
CREATE TABLE events (
    id VARCHAR NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    data JSONB,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Multi-tenant table
CREATE TABLE events (
    id VARCHAR NOT NULL,
    project_id VARCHAR NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    data JSONB,
    PRIMARY KEY (id, created_at, project_id)
) PARTITION BY RANGE (project_id, created_at);
```

## Features in Detail

### Partition Types

Currently supports:
- **Range Partitioning**: Time-based range partitioning with optional tenant ID support
- **List Partitioning**: Planned for future release
- **Hash Partitioning**: Planned for future release

### Maintenance Operations

- Automatically creates new partitions ahead of time based on `PartitionCount`
- Drops old partitions based on `RetentionPeriod`
- Supports custom pre-drop hooks for data archival or backup operations

### Multi-tenant Support

- Optional tenant-based partitioning using `TenantId` and `TenantIdColumn`
- Separate partition management per tenant
- Automatic partition naming with tenant ID inclusion

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT
