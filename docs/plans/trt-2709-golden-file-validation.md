# Golden File Validation for Partition Migration Queries

## Context

We've modified multiple query functions to add partition pruning filters (`prow_job_release`, `prow_job_run_release`, timestamp bounds) as part of TRT-2709 (phase 4b partitioning). These filters should be logically redundant with existing JOIN/WHERE conditions, meaning results must not change. We need to validate that the pre-migration codebase on an unpartitioned DB produces identical results to the post-migration codebase on a partitioned DB.

**Workflow:**
1. Commit current changes on `trt-2709-partitioning-phase2`
2. Switch to `master`, add golden file test, run `Test_GenerateGoldenFile` against unmodified staging DB
3. Switch back to `trt-2709-partitioning-phase2`, cherry-pick the golden file test
4. Update any validation cases that call renamed functions (e.g. `ProwJobRunIDs` -> `ProwJobRunCount`)
5. Run `Test_ValidateGoldenFile` against post-migration DB restored from the same backup

## New file

`pkg/flags/postgres_validation_test.go` (single file, same package as benchmarks)

## Data structures

```go
type validationSnapshot struct {
    RowCount   int               `json:"row_count"`
    SpotChecks map[string]string `json:"spot_checks,omitempty"`
}

type goldenFile struct {
    Metadata struct {
        AsOf      time.Time `json:"as_of"`
        Release   string    `json:"release"`
        Generated time.Time `json:"generated_at"`
    } `json:"metadata"`
    Results map[string]validationSnapshot `json:"results"`
}

type validationCase struct {
    name string
    fn   func(dbc *db.DB, asOf time.Time) (validationSnapshot, error)
}
```

## Validation cases

One `getValidationCases(asOf)` function returning `[]validationCase`, mirroring all cases from `getBenchmarkCases` + `getIndividualBenchmarkCases`. Each calls the same query function but captures a `validationSnapshot` with `RowCount` and a few `SpotChecks`.

**Spot check strategy per result type:**
- **Slice results** (JobReports, VariantReports, etc.): `row_count` + first element's identifying field (name/ID) after sorting
- **Scalar counts** (JobRunTestCount, ProwJobHistoricalTestCounts): value stored as `row_count`
- **PaginationResult** (JobsRunsReport, RecentTestFailures): `total_rows` as `row_count`
- **Map results** (TestAnalysisOverall, TestAnalysisByJob): number of keys as `row_count`, sorted key list as spot check
- **Special** (IsNewTestQuery): `found` bool, org/repo if found

Cases use the same constants (`benchmarkRelease`, `benchmarkJobName`, `benchmarkTestName`) and the same query parameters as benchmark cases. Uses the same `getBenchmarkDBClient` helper.

## Test functions

### `Test_GenerateGoldenFile`
- Env vars: `db_benchmarking_dsn` (existing), `golden_file_path` (required, path to write JSON)
- Records `asOf = time.Now().UTC()` and stores in golden file metadata
- Runs all validation cases, collects snapshots
- Writes `goldenFile` as indented JSON to `golden_file_path`

### `Test_ValidateGoldenFile`
- Env vars: `db_benchmarking_dsn`, `golden_file_path` (required, path to read JSON)
- Reads the golden file, extracts `asOf` from metadata to reuse the same time anchor
- Runs all validation cases with the stored `asOf`
- For each case, compares `RowCount` (exact match) and `SpotChecks` (exact string match per key)
- Reports all mismatches as test failures with clear diff output (expected vs got)
- Does NOT fail-fast: reports all mismatches before failing

## Time-sensitive queries

Some cases use `current_date` in raw SQL (TestCountsByLookback*, TestOutputs, TestDurations). Since both DBs are restored from the same backup, running both generate and validate on the same day keeps these consistent. The `asOf` parameter (used by Go-level date math like `PeriodToDates`) is stored and reused from the golden file.

## Cross-branch compatibility

On `master`, the validation case for job run counting calls `query.ProwJobRunIDs` and stores `len(ids)` as `row_count`. After cherry-picking to `trt-2709-partitioning-phase2`, update that one case to call `query.ProwJobRunCount` and store the count directly. The golden file format is identical either way, so comparison works.

## Verification

1. On `master`: `db_benchmarking_dsn=<staging_ro> golden_file_path=/tmp/golden.json go test -v -run Test_GenerateGoldenFile -count=1 ./pkg/flags/`
2. On `trt-2709-partitioning-phase2`: `db_benchmarking_dsn=<staging_ro> golden_file_path=/tmp/golden.json go test -v -run Test_ValidateGoldenFile -count=1 ./pkg/flags/`
3. All cases should pass with matching row counts and spot checks
