# Domain Concept: Job

**Type**: CI Job Execution  
**Data Source**: BigQuery (`prow.jobs` table)  
**Primary API**: `/api/jobs`

## Purpose

Represents a single execution of a Prow CI job. Jobs are the top-level unit of CI analysis in Sippy.

## Key Properties

| Property | Type | Description |
|----------|------|-------------|
| **Name** | string | Prow job name (e.g., `periodic-ci-openshift-release-master-nightly-4.16-e2e-aws`) |
| **Status** | enum | Success, Failure, Pending, Aborted, Error |
| **Duration** | duration | Job execution time |
| **Timestamp** | timestamp | Job start/end time |
| **Tests** | []Test | Individual test results within the job |
| **Variants** | map[string]string | Extracted NURP+ variant dimensions |

## Naming Convention

Prow jobs follow pattern: `<frequency>-ci-<org>-<repo>-<branch>-<type>-<variants>`

**Example**: `periodic-ci-openshift-release-master-nightly-4.16-e2e-aws`
- Frequency: `periodic`
- Type: `nightly`
- Release: `4.16`
- Platform: `aws`
- Test suite: `e2e`

## Job Lifecycle

1. **Triggered**: Prow scheduler creates job (periodic/presubmit/postsubmit)
2. **Running**: Job executes tests in cluster
3. **Completed**: Results uploaded to BigQuery
4. **Loaded**: Sippy dataloader imports to PostgreSQL
5. **Analyzed**: Statistics calculated, regressions detected

## Variant Extraction

Sippy parses job names to extract variant dimensions:

```go
// pkg/variantregistry
type Variant struct {
    Network  string // e.g., "sdn", "ovn"
    Upgrade  string // e.g., "upgrade", "micro"
    Platform string // e.g., "aws", "gcp", "azure"
    ...
}
```

See [variant.md](variant.md) for details.

## Database Schema

**Table**: `prow_job_run_tests`

```sql
CREATE TABLE prow_job_run_tests (
    id SERIAL PRIMARY KEY,
    job_name TEXT,
    test_name TEXT,
    status TEXT,
    duration INTERVAL,
    timestamp TIMESTAMPTZ,
    ...
);
```

## Common Queries

**Job pass rate**: `SELECT COUNT(*) FILTER (WHERE status='Success') / COUNT(*) FROM jobs WHERE name=?`

**Recent failures**: `SELECT * FROM jobs WHERE status='Failure' ORDER BY timestamp DESC LIMIT 10`

## Related Concepts

- [Test](test.md) - Individual test results within a job
- [Variant](variant.md) - Extracted dimensions from job name
- [Release](release.md) - OpenShift version being tested

## References

- API implementation: `pkg/api/jobs.go`
- Data loader: `pkg/dataloader/jobs.go`
- Database models: `pkg/db/models/job.go`
