# Domain Concept: Release

**Type**: OpenShift Version  
**Data Source**: Configuration + BigQuery  
**Primary API**: `/api/releases`

## Purpose

Represents an OpenShift version being tracked by Sippy. Releases are the primary organizational unit for CI analysis.

## Key Properties

| Property | Type | Description |
|----------|------|-------------|
| **Name** | string | Version (e.g., `4.16`, `4.17`) |
| **Status** | enum | `Active`, `GA`, `Prerelease`, `Archived` |
| **GA Date** | timestamp | General availability date |
| **Stream** | string | `nightly`, `ci`, `stable` |

## Release Lifecycle

1. **Prerelease**: Development phase (e.g., `4.17.0-0.nightly`)
2. **Feature Freeze**: No new features, stabilization
3. **Code Freeze**: Critical fixes only
4. **GA**: General availability (e.g., `4.17.0`)
5. **Stable**: Maintenance (z-stream releases like `4.17.1`)
6. **Archived**: No longer tracked

## Release Configuration

**File**: `config/releases.yaml` (example, actual config may differ)

```yaml
releases:
  - name: "4.16"
    ga_date: "2024-06-01"
    status: "GA"
    streams: ["nightly", "ci"]
  - name: "4.17"
    ga_date: "2025-01-15"
    status: "Prerelease"
    streams: ["nightly"]
```

## Component Readiness Views

When a release goes GA, Sippy generates component readiness views:

**Process**:
1. Run `/sippy-generate-release-views` to create views
2. Views track component health for the release
3. Run `/sippy-update-ga-release-views` when GA to update status

See [Component Readiness](component-readiness.md) for details.

## Database Schema

**Table**: `releases`

```sql
CREATE TABLE releases (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE,
    ga_date TIMESTAMPTZ,
    status TEXT,
    ...
);
```

## Common Queries

**Active releases**: `SELECT * FROM releases WHERE status IN ('Active', 'GA')`

**Jobs for release**: `SELECT * FROM jobs WHERE release='4.16'`

**Component readiness for release**: `SELECT * FROM component_readiness WHERE release='4.16'`

## Related Concepts

- [Variant](variant.md) - Release is a key variant dimension
- [Component Readiness](component-readiness.md) - Per-release component health tracking
- [Job](job.md) - Jobs are associated with releases

## References

- Release configuration: `config/`
- API implementation: `pkg/api/releases.go`
- Database models: `pkg/db/models/release.go`
