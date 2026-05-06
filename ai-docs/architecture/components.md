# Sippy Architecture

## System Overview

Sippy is a three-tier application:

```text
┌─────────────────┐
│   Frontend      │ React + Material-UI (sippy-ng)
│   (sippy-ng)    │
└────────┬────────┘
         │ HTTP/REST
┌────────▼────────┐
│   Backend       │ Go HTTP API (sippyserver)
│ (sippyserver)   │
└────────┬────────┘
         │
    ┌────┴────┬────────────┐
    │         │            │
┌───▼──┐ ┌───▼──┐ ┌───────▼─────┐
│ BQ   │ │ PG   │ │   Redis     │
│      │ │      │ │   (cache)   │
└──────┘ └──────┘ └─────────────┘
```

## Repository Structure

```text
sippy/
├── cmd/
│   └── sippy/           # Main entry point (HTTP server + CLI)
├── pkg/
│   ├── api/             # HTTP API handlers
│   ├── apis/            # Data structures (types)
│   ├── sippyserver/     # HTTP server setup
│   ├── dataloader/      # BigQuery → PostgreSQL ETL
│   ├── db/              # Database models and queries
│   ├── variantregistry/ # Variant extraction logic
│   ├── componentreadiness/ # Component analysis
│   ├── cache/           # Redis caching
│   └── ...
├── sippy-ng/            # React frontend
│   └── src/
│       ├── component_readiness/
│       ├── releases/
│       ├── jobs/
│       └── ...
├── test/
│   └── e2e/             # End-to-end API tests
└── scripts/             # Deployment and utility scripts
```

## Backend Components

### HTTP API Server (pkg/sippyserver)

**Responsibility**: Serve REST API for frontend

**Key files**:
- `pkg/sippyserver/server.go` - HTTP server setup
- `pkg/api/` - API endpoint handlers

**Endpoints**: See `pkg/api/README.md`

### Data Loader (pkg/dataloader)

**Responsibility**: Import BigQuery data into PostgreSQL

**Process**:
1. Query BigQuery for recent job results
2. Parse and normalize test names
3. Extract variants from job names
4. Insert into PostgreSQL tables

**Key files**:
- `pkg/dataloader/loader.go` - Main loader logic
- `pkg/dataloader/bigquery.go` - BigQuery client

**Execution**: Run via `/sippy-dev-regression-cache` skill or `sippy load-data`

### Variant Registry (pkg/variantregistry)

**Responsibility**: Define and extract variants from job names

**Pattern matching**:
```go
type VariantDefinition struct {
    Name    string
    Pattern *regexp.Regexp
    Values  []string
}
```

**Key files**: `pkg/variantregistry/registry.go`

### Component Readiness (pkg/componentreadiness)

**Responsibility**: Calculate component health statistics

**Key operations**:
- Aggregate test results by component
- Calculate pass rates
- Detect regressions
- Generate component readiness views

**Key files**:
- `pkg/componentreadiness/calculator.go`
- `pkg/componentreadiness/component_mapping.go`

### Database Layer (pkg/db)

**Responsibility**: PostgreSQL schema and queries

**Tables**:
- `prow_job_run_tests` - Job results
- `prow_job_run_test_outputs` - Test results
- Component readiness views (e.g., `component_readiness_4_16`)

**Migrations**: See [SIPPY_DEVELOPMENT.md](../SIPPY_DEVELOPMENT.md#database-migrations)

### Cache Layer (pkg/cache)

**Responsibility**: Redis caching for expensive queries

**Cached data**:
- Component readiness results
- Job statistics
- Test pass rates

**TTL**: Configurable per query type

## Frontend Components

### React Application (sippy-ng)

**Technology**: React 18 + Material-UI + React Router

**Key modules**:
- `component_readiness/` - Component health dashboard
- `releases/` - Release overview pages
- `jobs/` - Job result browser
- `datagrid/` - Reusable table components

**Build**: `make frontend` (see [SIPPY_DEVELOPMENT.md](../SIPPY_DEVELOPMENT.md))

## Data Flow

### Read Path (API Query)

```text
1. User requests /api/componentreadiness?release=4.16
2. API handler checks Redis cache
3. Cache miss → Query PostgreSQL view
4. Return JSON response
5. Cache result in Redis (TTL: 5 min)
```

### Write Path (Data Loader)

```text
1. Cron job triggers data loader
2. Query BigQuery for new job results
3. Extract variants from job names
4. Normalize test names
5. Insert into PostgreSQL
6. Invalidate relevant Redis cache entries
7. Update component readiness views (if needed)
```

## Deployment

**Environments**:
- Production: OpenShift cluster
- Development: Local (see [SIPPY_DEVELOPMENT.md](../SIPPY_DEVELOPMENT.md))

**Skills**:
- `/sippy-dev-app` - Start backend + frontend dev servers
- `/sippy-dev-serve` - Backend only
- `/sippy-dev-frontend` - Frontend only
- `/sippy-dev-migrate` - Run database migrations
- `/sippy-dev-regression-cache` - Load BigQuery data

## Related Documentation

- [Domain concepts](../domain/) - CI analysis domain model
- [SIPPY_DEVELOPMENT.md](../SIPPY_DEVELOPMENT.md) - Development workflows
- [SIPPY_TESTING.md](../SIPPY_TESTING.md) - Test suites
- [API documentation](../../pkg/api/README.md) - API endpoints
