# Domain Concepts

Sippy's domain model centers on CI analysis concepts, not Kubernetes CRDs.

## Core Concepts

- [job.md](job.md) - Prow CI job execution (top-level unit of analysis)
- [test.md](test.md) - Individual test case within a job (atomic unit of CI signal)
- [variant.md](variant.md) - NURP+ dimensions for slicing data (Network, Upgrade, Release, Platform, etc.)
- [release.md](release.md) - OpenShift version being tracked
- [component-readiness.md](component-readiness.md) - Statistical assessment of component health

## Concept Relationships

```text
Release (4.16)
    │
    ├── Job (periodic-ci-...-4.16-e2e-aws)
    │       ├── Variants (platform:aws, release:4.16)
    │       └── Tests
    │               ├── Test 1 (passed)
    │               ├── Test 2 (failed)
    │               └── Test 3 (passed)
    │
    └── Component Readiness
            ├── Networking (95% pass rate)
            ├── Storage (92% pass rate)
            └── API (98% pass rate)
```

## Data Flow

1. **Prow** executes jobs → Results to BigQuery
2. **Sippy dataloader** imports BigQuery → PostgreSQL
3. **Variant extraction** from job names → Variants table
4. **Test aggregation** → Component readiness views
5. **Regression detection** → Alerts

## Navigation

- For architecture details: See [../architecture/](../architecture/)
- For development workflows: See [../SIPPY_DEVELOPMENT.md](../SIPPY_DEVELOPMENT.md)
- For API details: See [../../pkg/api/README.md](../../pkg/api/README.md)
