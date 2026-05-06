# Domain Concept: Test

**Type**: Test Case Execution  
**Data Source**: BigQuery (extracted from junit XMLs)  
**Primary API**: `/api/tests`

## Purpose

Represents a single test case execution within a job. Tests are the atomic unit of CI signal analysis.

## Key Properties

| Property | Type | Description |
|----------|------|-------------|
| **Name** | string | Test identifier (e.g., `[sig-network] should allow traffic`) |
| **Status** | enum | Passed, Failed, Skipped, Flake |
| **Duration** | duration | Test execution time |
| **Job** | Job | Parent job containing this test |
| **FailureMessage** | string | Error message if failed |

## Test Identification

Tests are identified by normalized name across jobs:

```go
// pkg/testidentification
type TestIdentifier struct {
    Name      string // Normalized name
    Suite     string // e.g., "openshift-tests", "kubernetes"
    Component string // e.g., "Networking", "Storage"
}
```

**Normalization**: Removes timestamps, UUIDs, cluster-specific details to enable cross-job aggregation.

## Test Lifecycle

1. **Executed**: Test runs in CI job
2. **Reported**: Result written to junit XML
3. **Uploaded**: junit XML uploaded to GCS
4. **Parsed**: BigQuery parses junit XML
5. **Loaded**: Sippy imports to PostgreSQL
6. **Aggregated**: Statistics calculated across jobs/variants

## Pass Rate Calculation

**Formula**: `pass_rate = passed_count / (passed_count + failed_count)`

**Skipped tests**: Excluded from pass rate calculation

**Flakes**: Tests that sometimes pass, sometimes fail (tracked separately)

## Regression Detection

Sippy compares current pass rate vs historical baseline:

```go
if currentPassRate < (historicalPassRate - threshold) {
    // Flag as regression
}
```

**Threshold**: Configurable per release (default: 5% drop)

## Database Schema

**Table**: `prow_job_run_test_outputs`

```sql
CREATE TABLE prow_job_run_test_outputs (
    id SERIAL PRIMARY KEY,
    prow_job_run_test_id INTEGER REFERENCES prow_job_run_tests(id),
    test_name TEXT,
    status TEXT,
    duration INTERVAL,
    failure_message TEXT,
    ...
);
```

## Common Queries

**Test pass rate**: `SELECT COUNT(*) FILTER (WHERE status='Passed') / COUNT(*) FROM tests WHERE name=?`

**Flaky tests**: `SELECT name, COUNT(DISTINCT status) FROM tests GROUP BY name HAVING COUNT(DISTINCT status) > 1`

**Top failures**: `SELECT name, COUNT(*) FROM tests WHERE status='Failed' GROUP BY name ORDER BY COUNT DESC LIMIT 10`

## Related Concepts

- [Job](job.md) - Parent job execution
- [Variant](variant.md) - Test results sliced by variant
- [Component Readiness](component-readiness.md) - Aggregated test statistics

## References

- API implementation: `pkg/api/tests.go`
- Test identification: `pkg/testidentification/`
- Database models: `pkg/db/models/test.go`
