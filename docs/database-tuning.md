# Database Tuning

This document is the source of truth for PostgreSQL parameter group
customizations required by Sippy. These settings must be configured in the
RDS parameter group (or equivalent server-level configuration) and are not
controlled by the application code.

Session-level parameters set by the application on each connection are in
`pkg/db/db.go`. Per-query transaction overrides are in the query code itself
(look for `SET LOCAL`). This document covers only the server-level settings
that live outside the codebase.

## RDS Parameter Group Settings

Differences from the default `default.postgres14` parameter group:

| Parameter | Value | Default | Rationale |
|-----------|-------|---------|-----------|
| `effective_io_concurrency` | 200 | 1 | Number of concurrent I/O operations the OS can handle. Set high for SSD/NVMe-backed storage (gp3, io1). |
| `max_parallel_workers` | 16 | `GREATEST({DBInstanceVCPU/2},8)` | Total parallel workers available across all concurrent queries. Component readiness queries request 4 workers each via `SET LOCAL`, and multiple queries run concurrently. Fixed to avoid dependence on instance vCPU count. |
| `random_page_cost` | 1.1 | 4.0 | Favors index scans over sequential scans, appropriate for SSD/NVMe storage. Also set per-session by the application in `pkg/db/db.go`. |
