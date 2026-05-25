# Proposal: Add Release Clustering to the JUnit BigQuery Table

## Problem

The `ci_analysis_us.junit` table (9.3 billion rows, 5.76 TB) is partitioned by `modified_time` (day) but has no clustering. Every query scans all releases within the requested date range, even when only one release is needed.

Over the past 30 days:

| Metric | Value |
|--------|-------|
| Total queries | 55,105 |
| Total data scanned | 5,816 TB |
| Estimated cost (on-demand @ $6.25/TB) | **$36,351/month** (~$436K/year) |

97% of that cost ($35,189) comes from a single query pattern: sippy's deduplication + variant join query, run ~51,000 times per month. Every one of these queries already carries `@BaseRelease` and `@SampleRelease` parameters, but the release filtering is applied **after** the full table scan — via a JOIN to `job_variants`. BigQuery has no way to skip irrelevant data blocks.

### Data distribution (30 days)

| Branch | % of rows |
|--------|-----------|
| 4.22 | 33.85% |
| 5.0 | 33.22% |
| 4.21 | 11.27% |
| 4.20 | 5.13% |
| All others | 16.53% |

A sippy query targeting release 4.22 reads 334.6 GB but only needs ~34% of it.

### Top consumers

| Consumer | Queries/month | Cost |
|----------|--------------|------|
| sippy-bigquery-job-importer | 35,362 | $21,942 |
| openshift-trt-sippy-consumer | 18,821 | $13,307 |
| ci-infra-users-to-bigquery | 55 | $764 |
| All human users | ~867 | $338 |

## Proposed Changes

### Step 1: Add a `release` column and cluster on it

Add a new `release` column to the junit table populated from the variant registry's `Release` variant value. Cluster the table on this column.

**Why a new column instead of using the existing `branch` column?**

The `branch` column records the *source repo branch*, not the *OCP release under test*. It is derived in the ingestion Cloud Function (`ci-cloud-functions/ci-to-bigquery/gcs_finalize_event.py`) via a regex heuristic against the prowjob name:

```python
branch_pattern = re.compile(r".*?\D+[-\/](\d\.\d+)\D?.*|.*\-(master|main)\-.*")
```

This regex extracts the first version-like string (e.g. `4.22`) from the job name, or falls back to `master`/`main`, or sets `unknown` if neither matches. It is fundamentally a heuristic that cannot reliably determine the OCP release under test. Analysis of the past 30 days shows a 4.55% mismatch rate (274 of 6,021 job names) between `branch` and the authoritative `Release` variant from the variant registry.

Clustering on `branch` would silently miss these rows in filtered queries. The mismatches fall into predictable categories, each directly explained by the regex behavior:

**Category 1: Operators built from `main`/`master`, tested against a specific OCP release**

The regex matches `-main-` or `-master-` and returns that literal string. The actual OCP release under test (embedded elsewhere in the job name or only known to the variant registry) is lost:

| Job | `branch` | `Release` variant |
|-----|----------|-------------------|
| `periodic-ci-openshift-cluster-kube-apiserver-operator-main-periodics-e2e-aws-encryption-kms` | main | 5.0 |
| `periodic-ci-openshift-multiarch-tuning-operator-main-ocp419-e2e-aws-ovn-proxy-mto-origin` | main | 4.19 |
| `periodic-ci-openshift-kni-eco-ci-cd-main-cnf-network-phase1-4.22-cnf-network-functional-tests` | main | 4.22 |
| `periodic-ci-openshift-ovn-kubernetes-master-e2e-aws-ovn-local-to-shared-gateway-mode-migration-periodic` | master | 5.0 |
| `periodic-ci-openshift-operator-framework-olm-main-periodics-e2e-gcp-ovn` | main | 4.17 |

**Category 2: Branch derivation failures (`unknown`)**

The regex requires a non-digit character before the version (`\D+[-\/]`). Job names containing `release-v1.21` have a `v` prefix on the version that breaks the pattern. Names like `scos-stable` contain no version-like string at all. Both fall through to `unknown`:

| Job | `branch` | `Release` variant |
|-----|----------|-------------------|
| `periodic-ci-openshift-knative-serving-release-v1.21-421-test-e2e-c` | unknown | 1.21 |
| `periodic-ci-openshift-knative-eventing-kafka-broker-release-v1.16-420-test-e2e-c` | unknown | 1.16 |
| `periodic-ci-openshift-pipelines-release-tests-release-v1.22-openshift-pipelines-ocp4.21-lp-rosa-hypershift-aws-rosa-hypershift` | unknown | 4.21 |
| `periodic-ci-openshift-multiarch-tuning-operator-v1.x-ocp418-e2e-aws-ovn-proxy-mto-origin` | unknown | 4.18 |
| `release-openshift-okd-scos-installer-e2e-aws-upgrade-from-scos-stable` | unknown | 4.22 |

**Category 3: Operator-versioned branches (own versioning scheme, not OCP)**

The regex captures the *first* version-like string in the job name. For `oadp-1.6-4.22-...`, that's `1.6` (the operator version), not `4.22` (the OCP release). Multi-hop upgrade jobs like `upgrade-4.19-to-4.20-to-4.21-to-4.22` similarly capture the first version (`4.19`) rather than the target (`4.22`):

| Job | `branch` | `Release` variant |
|-----|----------|-------------------|
| `periodic-ci-openshift-oadp-operator-oadp-1.6-4.22-e2e-test-aws-periodic` | 1.6 | 4.22 |
| `periodic-ci-openshift-oadp-operator-oadp-1.5-4.20-e2e-test-kubevirt-aws-periodic` | 1.5 | 4.20 |
| `periodic-ci-openshift-knative-serverless-operator-release-1.37-ocp-4.22-lp-interop-aws-fips` | 1.37 | 4.22 |
| `periodic-ci-openshift-service-mesh-sail-operator-release-3.3-ocp-4.20-sync-upstream` | 3.3 | 4.20 |
| `release-openshift-origin-installer-e2e-aws-upgrade-4.19-to-4.20-to-4.21-to-4.22-ci` | 4.19 | 4.22 |
| `periodic-ci-openshift-assisted-test-infra-release-ocm-2.15-e2e-metal-assisted-ha-kube-api-4-20-periodic` | 2.15 | 4.20 |

**Category 4: Managed service / non-version release variants**

The regex correctly extracts a version (e.g. `4.22` from `nightly-4.22-e2e-rosa-hcp-ovn`), but the variant registry intentionally maps these jobs to a deployment target like `rosa-stage` rather than an OCP version. The regex has no way to know this distinction:

| Job | `branch` | `Release` variant |
|-----|----------|-------------------|
| `periodic-ci-openshift-release-main-nightly-4.22-e2e-rosa-hcp-ovn` | 4.22 | rosa-stage |
| `periodic-ci-openshift-release-main-nightly-5.0-e2e-rosa-sts-ovn` | 5.0 | rosa-stage |
| `periodic-ci-Azure-ARO-HCP-main-periodic-stage-e2e-parallel` | main | aro-stage |
| `periodic-ci-Azure-ARO-HCP-main-periodic-prod-e2e-parallel` | main | aro-production |
| `periodic-ci-openshift-hypershift-release-4.20-periodics-hcm-e2e-aws` | 4.20 | ocp-hypershift |
| `periodic-ci-openshift-hypershift-main-periodic-jira-agent` | main | automation |
| `periodic-ci-openshift-online-rosa-regional-platform-main-nightly-integration` | main | rrp-integration |

### Step 2: Update sippy queries to filter on `release`

Add a predicate to the WHERE clause on the junit table. Sippy queries use both `@BaseRelease` and `@SampleRelease` parameters depending on context:

```sql
AND release = @BaseRelease
-- or
AND release = @SampleRelease
```

This enables BigQuery's block-level pruning on the clustered column.

### Step 3: Update the ingestion pipeline

The Cloud Function that writes to the junit table needs to populate the `release` column by looking up the job's `Release` variant at insert time.

## Implementation Sequence

Steps 1-3 are independent and can ship on different timelines:

1. **ALTER TABLE to add clustering** — immediate, zero-risk DDL. New data is clustered on write. Existing data is auto-reclustered by BigQuery in the background over days/weeks.

   ```sql
   ALTER TABLE `openshift-gce-devel.ci_analysis_us.junit`
   ADD COLUMN release STRING;

   ALTER TABLE `openshift-gce-devel.ci_analysis_us.junit`
   SET OPTIONS(clustering_columns=['release']);
   ```

2. **Update the ingestion pipeline** — deploy whenever ready. Add an in-memory cache of the `job_name → release` mapping from the variant registry (see [Ingestion Pipeline: Release Lookup](#ingestion-pipeline-release-lookup) below). From this point forward, new rows have a populated `release` column and are physically clustered by it.

3. **Update sippy queries** — add `AND release = @BaseRelease`. The moment this ships, queries start benefiting from however much data has been clustered since step 1.

4. **One-time historical backfill** (required) — populate `release` for all existing rows via a CTAS rebuild. Sippy's Component Readiness queries compare across historical release windows, so all existing data must have the `release` column populated for the clustering benefit to apply.

## Expected Savings

When a query filters on a clustered column, BigQuery skips data blocks that don't contain matching values. For single-release queries:

- A 4.22 query scans ~34% instead of 100% → **66% reduction**
- A 5.0 query scans ~33% instead of 100% → **67% reduction**
- A 4.21 query scans ~11% instead of 100% → **89% reduction**

| Metric | Current | With Clustering |
|--------|---------|-----------------|
| Avg bytes/query | 108.6 GB | ~27-43 GB |
| Monthly scan | 5,816 TB | ~1,450-2,300 TB |
| Monthly cost | $36,351 | ~$9,100-$14,400 |
| **Annual savings** | | **$264K-$327K** |

## Historical Backfill (Required)

Sippy's Component Readiness queries compare test results across historical release windows (e.g. 4.21 GA period vs current 4.22), so all existing data must have the `release` column populated.

### CTAS rebuild (~$6, one statement, brief ingestion pause)

Create a new table with `release` pre-populated by joining to `job_variants`, then rename:

```sql
CREATE TABLE `ci_analysis_us.junit_v2`
PARTITION BY DATE(modified_time)
CLUSTER BY release
OPTIONS(partition_expiration_days=1460, require_partition_filter=true)
AS
SELECT j.*, jv.variant_value AS release
FROM `ci_analysis_us.junit` j
LEFT JOIN `ci_analysis_us.job_variants` jv
  ON j.prowjob_name = jv.job_name
  AND jv.variant_name = "Release";
```

Then:
```sql
ALTER TABLE `ci_analysis_us.junit` RENAME TO `ci_analysis_us.junit_old`;
ALTER TABLE `ci_analysis_us.junit_v2` RENAME TO `ci_analysis_us.junit`;
```

**Tradeoff:** Requires pausing the ingestion Cloud Function for 10-30 minutes while the CTAS runs and the rename completes. Any junit data from Prow jobs that complete during this window will be permanently lost — the ingestion pipeline does not replay missed data. The impact is a small gap in test results for jobs that happened to finish during the maintenance window.

## Ingestion Pipeline: Release Lookup

The ingestion Cloud Function (`gcs_finalize_event.py`) fires on every GCS object write — nearly continuously. It needs an efficient way to look up the variant registry `Release` value for each job without querying BigQuery on every invocation.

### Approach: In-memory cache loaded on cold start

The `job_variants` table contains 24,839 job-to-release mappings totaling ~1.9 MB on disk. As a Python dict with string keys and values, this consumes approximately **4-5 MB of memory** (Python string overhead of ~50 bytes per string, plus dict hash table overhead, for ~25K entries).

The Cloud Function already uses this exact pattern for its BigQuery and GCS clients — module-level globals initialized once and reused across invocations on the same instance:

```python
# Existing pattern in gcs_finalize_event.py (lines 125-155):
global_bq_client = None

def process_connection_setup(bucket: str):
    global global_bq_client
    if not global_storage_client:          # only runs on cold start
        global_bq_client = bigquery.Client(...)
```

The release cache follows the same pattern, with a 6-hour TTL as a safety net in case an instance lives unusually long (the variant registry only refreshes once per day, so stale data is at most ~24 hours behind):

```python
import time
from typing import Optional

# Module-level cache — persists across invocations on the same instance
_release_cache: dict = {}
_cache_loaded_at: float = 0
_CACHE_TTL_SECONDS = 6 * 60 * 60  # 6 hours; variant registry refreshes daily

def _load_release_cache():
    """Load the full job_name -> release mapping from job_variants.
    Queries ~50 MB from BigQuery, costs ~$0.0003 per load.
    """
    global _release_cache, _cache_loaded_at
    client = bigquery.Client(project="openshift-gce-devel")
    rows = client.query(
        'SELECT job_name, variant_value '
        'FROM `openshift-gce-devel.ci_analysis_us.job_variants` '
        'WHERE variant_name = "Release"'
    ).result()
    _release_cache = {row.job_name: row.variant_value for row in rows}
    _cache_loaded_at = time.time()

def get_release_for_job(job_name: str) -> Optional[str]:
    """Look up the release for a job name. Returns None if not in the registry."""
    if not _release_cache or (time.time() - _cache_loaded_at > _CACHE_TTL_SECONDS):
        _load_release_cache()
    return _release_cache.get(job_name)
```

Usage at the point where junit records are created (around line 1329):

```python
record = JUnitTestRecord(
    ...
    branch=self.branch,                          # keep existing field
    release=get_release_for_job(self.prowjob_name) or self.branch,  # new field
    ...
)
```

### Cost and performance characteristics

| Metric | Value |
|--------|-------|
| Cache size in memory | ~4-5 MB (25K string pairs + dict overhead) |
| BigQuery cost per cache load | ~$0.0003 (50 MB table scan) |
| Cache loads per instance lifetime | 1 (cold start); rarely 2+ if instance lives >6 hours |
| Lookup latency (warm) | dict lookup, effectively zero |
| Fallback for unknown jobs | Uses regex-derived `branch` value (existing behavior) |

### Why not GCS or an external cache?

The variant registry could write a JSON mapping file to GCS, but the Cloud Function already has a BigQuery client initialized on cold start. Adding a 50 MB query to that initialization path is simpler than introducing a new GCS file that needs to be generated and maintained. The cost difference is negligible.

## Handling Variant Registry Release Changes

When the variant registry corrects a job's Release assignment, existing junit rows have a stale `release` value. This is expected to be rare (a few times a year at most).

**Detection:** The variant registry update job can track when a job's Release variant changes and emit the affected job names.

**Correction:** A batch update script (`update_junit_release.py`) handles the BigQuery DML constraints:

- DML UPDATE rewrites entire partitions, so even updating one job scans the full table for the affected date range
- BigQuery limits DML to ~100 GB modified per statement and 20 statements per table per day
- The script groups partitions into batches that stay under these limits

Usage:
```bash
# Preview
python update_junit_release.py --dry-run \
    --job-name "periodic-ci-some-job" \
    --new-release "4.23"

# Execute
python update_junit_release.py \
    --job-name "periodic-ci-some-job" \
    --new-release "4.23"

# Batch from variant registry change detector
python update_junit_release.py --dry-run --jobs-file changed_jobs.json
```

Estimated cost per correction: ~$6 for a full-history update of a single job. In practice, most corrections only need 30-90 days of recent data, costing ~$2-4.

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| ALTER TABLE breaks existing queries | None | Clustering is invisible to queries that don't filter on the clustered column. Adding a column doesn't affect existing SELECTs. |
| Ingestion pipeline disruption | Low | Column addition and clustering are metadata-only DDL. The pipeline change is additive (populate one new column). |
| `release` column has stale values after variant registry fix | Low | Rare event. Batch update script handles it. Sippy continues to JOIN on `job_variants` as a fallback — the `release` filter is additive pruning, not a correctness requirement. |
| Auto-reclustering takes too long | Low | New data (most queried) is clustered immediately. Historical data reclusters in background. CTAS rebuild available if needed. |
| Clustering doesn't achieve projected savings | Low | BigQuery clustering on a low-cardinality column (~10 significant values) with large data volumes is a well-understood optimization. Actual savings will be visible in `INFORMATION_SCHEMA.JOBS` within days of shipping the query change. |

## Experimental Validation (2026-05-25)

To validate the clustering hypothesis before committing to schema changes, we created a 90-day copy of the junit table clustered on the existing `branch` column:

```sql
CREATE TABLE `ci_analysis_us.junit_clustered_branch_test`
PARTITION BY DATE(modified_time)
CLUSTER BY branch
OPTIONS(partition_expiration_days=90, require_partition_filter=true)
AS
SELECT *
FROM `ci_analysis_us.junit`
WHERE modified_time >= DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 90 DAY);
```

We then ran a representative Component Readiness query (30-day window, release 4.22, with standard variant grouping) against both tables. The query was derived from `BuildComponentReportQuery` in `pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go`, with an added `AND branch = '4.22'` predicate in the junit scan CTE.

**Key finding:** BigQuery dry-run estimates do not account for clustering pruning — both tables reported the same upper bound (~214 GB). Only actual execution reveals the difference.

### Results

| | Original `junit` | Clustered on `branch` |
|---|---|---|
| **Bytes processed** | 214.46 GB | 70.60 GB |
| **Cost** | $1.34 | $0.44 |
| **Reduction** | — | **67%** |
| **Duration** | 7s | 7s |

The 67% reduction matches the prediction: release 4.22 represents ~34% of rows, so filtering on `branch` with clustering prunes the remaining ~66% at the block level.

### Cost of the experiment

| Item | Cost |
|------|------|
| CTAS copy (90 days, 861 GB scanned) | $5.38 |
| Query against original table | $1.34 |
| Query against clustered table | $0.44 |
| **Total** | **$7.16** |

The test table (`junit_clustered_branch_test`) has `partition_expiration_days=90` and will auto-clean.

## Cost Summary

| Item | Cost |
|------|------|
| ALTER TABLE (add column + clustering) | Free |
| Historical backfill (CTAS, required) | ~$6 one-time |
| Variant registry release corrections | ~$6 per event (rare) |
| **Monthly savings after full rollout** | **$22,000-$27,000** |
