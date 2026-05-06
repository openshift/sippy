# Sippy - Development Guide

> **Generic Development Practices**: See [Tier 1 Development Practices](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development) for Go standards, API design, and CI/CD workflows.

> **Detailed Setup**: See [../DEVELOPMENT.md](../DEVELOPMENT.md) for comprehensive instructions.

This guide covers **Sippy-specific** development workflows for AI agents.

## Quick Start

### Prerequisites

```text
Tool            | Version | Purpose
----------------|---------|------------------------------------------
Go              | 1.23+   | Backend development
Node.js         | 20+     | Frontend development (sippy-ng)
PostgreSQL      | 16+     | Data storage
Redis           | 7+      | Query caching
GCP credentials | -       | BigQuery access (optional for dev)
```

### Devcontainer (Recommended)

Use `.devcontainer/` for pre-configured environment with all tools. See [../.devcontainer/README.md](../.devcontainer/README.md).

**MCP Tools**: Use `/sippy-dev-*` skills for common tasks (see [../mcp/README.md](../mcp/README.md)).

## Repository Structure

```text
sippy/
├── cmd/sippy/              # Main CLI (server + data loader + migrations)
├── pkg/
│   ├── api/                # HTTP API handlers (REST endpoints)
│   ├── apis/               # Data structures (API types)
│   ├── sippyserver/        # HTTP server setup
│   ├── dataloader/         # BigQuery → PostgreSQL ETL
│   ├── db/                 # Database models, queries, migrations
│   ├── variantregistry/    # Variant extraction (NURP+ model)
│   ├── componentreadiness/ # Component health analysis
│   ├── cache/              # Redis caching layer
│   └── ...
├── sippy-ng/               # React frontend (Material-UI)
│   └── src/
│       ├── component_readiness/
│       ├── releases/
│       ├── jobs/
│       └── ...
├── test/e2e/               # API end-to-end tests
└── scripts/                # Deployment and utility scripts
```

## Build Commands

| Command | Output | Purpose |
|---------|--------|---------|
| `make` | `./sippy` | All-in-one binary (backend + embedded frontend) |
| `make sippy` | `./sippy` | Backend only |
| `make frontend` | `sippy-ng/build/` | Frontend build (production) |
| `make test` | - | Unit tests (Go + Jest) |
| `make lint` | - | Linters (golangci-lint + eslint) |
| `make e2e` | - | E2E API tests (⚠️ expensive BigQuery queries) |

**Important**: Never run `make e2e` more than once per request. See [SIPPY_TESTING.md](SIPPY_TESTING.md#e2e-tests).

## Development Workflows

### Backend Development

**Run server**:
```bash
/sippy-dev-serve         # MCP skill (recommended)
# or
./sippy serve --database-dsn=$SIPPY_DATABASE_DSN
```

**Database migrations**:
```bash
/sippy-dev-migrate       # MCP skill (recommended)
# or
./sippy migrate --database-dsn=$SIPPY_DATABASE_DSN
```

**Load data from BigQuery**:
```bash
/sippy-dev-regression-cache   # MCP skill (recommended)
# or
./sippy load --release=4.16 --config=./config/openshift.yaml
```

### Frontend Development

**Run dev server**:
```bash
/sippy-dev-frontend      # MCP skill (recommended)
# or
cd sippy-ng && npm start
```

**Access**: `http://localhost:3000` (proxies API to backend)

### Full Stack Development

**Run both backend + frontend**:
```bash
/sippy-dev-app           # MCP skill (recommended)
```

Backend: `http://localhost:8080`  
Frontend: `http://localhost:3000`

## Database

**Schema**: Managed via migrations in `pkg/db/migrations/`

**Key tables**:
- `prow_job_run_tests` - Job results
- `prow_job_run_test_outputs` - Test results
- Component readiness views (e.g., `component_readiness_4_16`)

**Migrations**: See [../CLAUDE.md](../CLAUDE.md#database-migration)

**Default DSN**: `postgresql://postgres:password@localhost:5432/postgres`

## Common Tasks

### Add New API Endpoint

1. Define handler in `pkg/api/[feature].go`
2. Register route in `pkg/sippyserver/server.go`
3. Add types to `pkg/apis/api/types.go`
4. Add E2E test in `test/e2e/`
5. Update API docs in `pkg/api/README.md`

### Add New Variant

1. Define pattern in `pkg/variantregistry/registry.go`
2. Update variant snapshot: `make update-variants`
3. Add frontend filter in `sippy-ng/src/components/VariantSelector.js`
4. Test variant extraction with existing jobs

### Generate Component Readiness Views

**For new release**:
```bash
/sippy-generate-release-views    # Generates views for new release
```

**When release goes GA**:
```bash
/sippy-update-ga-release-views   # Updates GA status
```

See [domain/component-readiness.md](domain/component-readiness.md) for details.

### Update Job Variant

**Interactive update**:
```bash
/sippy-update-job-variant        # MCP skill
```

## Testing

See [SIPPY_TESTING.md](SIPPY_TESTING.md) for comprehensive testing guide.

**Quick commands**:
```bash
make test           # Unit tests (Go + Jest)
make lint           # Linters
make e2e            # E2E tests (⚠️ run once only)
/sippy-dev-tests    # MCP skill (runs lint + unit + e2e)
```

## Debugging

**Backend logs**: Server outputs to stdout

**Frontend logs**: Browser console (React DevTools)

**Database queries**: Set `SIPPY_LOG_LEVEL=debug`

**Redis cache**: Use `redis-cli` to inspect cached keys

## Component-Specific Notes

**BigQuery credentials**: Optional for local dev. Use prod backup instead (see [../DEVELOPMENT.md](../DEVELOPMENT.md#from-a-prod-sippy-backup)).

**Variant snapshot**: Auto-generated `pkg/variantregistry/snapshot.yaml` must be kept in sync. Run `make update-variants` after variant logic changes.

**Component readiness views**: Generated per-release. Must create views before querying (use `/sippy-generate-release-views`).

**E2E tests**: Query live BigQuery. Expensive. Never run more than once. See [SIPPY_TESTING.md](SIPPY_TESTING.md#e2e-tests).

## See Also

- [SIPPY_TESTING.md](SIPPY_TESTING.md) - Test suites and patterns
- [architecture/components.md](architecture/components.md) - Sippy internals
- [../DEVELOPMENT.md](../DEVELOPMENT.md) - Detailed setup guide
- [../pkg/api/README.md](../pkg/api/README.md) - API documentation
- [Tier 1 Development Practices](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development)
