# TRT-2782: Provide test_details HATEOAS links for all tests (Phase 1)

## Context

The component_readiness API only includes test_details HATEOAS links for regressed tests. Two code paths enforce this restriction:

1. `getNewCellStatus` only adds tests to `regressedTests` when `ReportStatus < MissingSample` (-100). The `PostAnalysis` middleware loop only iterates over `RegressedTests`.
2. `LinkInjector.PostAnalysis` has an early return for `ReportStatus > FixedRegression` (-150).

This means non-regressed cells (green/grey/blue) have no test_details links, preventing navigation to individual test results.

## Approach

Add a new `AllTests` field alongside the existing `RegressedTests`. This preserves backward compatibility while exposing all analyzed tests with HATEOAS links. Remove the LinkInjector status guard so links are generated for every test status.

All other middleware PostAnalysis methods already handle non-regressed tests safely:
- `RegressionTracker.PostAnalysis` returns early at line 124 (`ReportStatus > SignificantTriagedRegression`)
- `RegressionAllowances.PostAnalysis` is a no-op (returns nil)

Regressed tests appear in both lists and get middleware-processed twice. This is safe because all middleware operations are idempotent (map key assignment, DB lookup with early return).

## Changes

### 1. Add `AllTests` to response type
**File:** `pkg/apis/api/componentreport/types.go`

Add `AllTests []ReportTestSummary` field to `ReportColumn`:
```go
type ReportColumn struct {
    crtest.ColumnIdentification
    Status         crtest.Status       `json:"status"`
    RegressedTests []ReportTestSummary `json:"regressed_tests,omitempty"`
    AllTests       []ReportTestSummary `json:"all_tests,omitempty"`
}
```

### 2. Add `allTests` to internal `cellStatus` struct
**File:** `pkg/api/componentreadiness/component_report.go` (line 506)

```go
type cellStatus struct {
    status         crtest.Status
    regressedTests []crtype.ReportTestSummary
    allTests       []crtype.ReportTestSummary
}
```

### 3. Populate `allTests` unconditionally in `getNewCellStatus`
**File:** `pkg/api/componentreadiness/component_report.go` (line 511)

- Carry forward `existingCellStatus.allTests` (like we do for `regressedTests`)
- Always create a `ReportTestSummary` and append to `allTests`
- Keep the existing conditional append to `regressedTests` unchanged

### 4. Assign `AllTests` in `buildReport`
**File:** `pkg/api/componentreadiness/component_report.go` (line 726)

After the existing `reportColumn.RegressedTests = status.regressedTests`, add:
```go
reportColumn.AllTests = status.allTests
sort.Slice(reportColumn.AllTests, func(i, j int) bool {
    return reportColumn.AllTests[i].ReportStatus < reportColumn.AllTests[j].ReportStatus
})
```

### 5. Run PostAnalysis middleware over `AllTests`
**File:** `pkg/api/componentreadiness/component_report.go` (line 125)

After the existing `RegressedTests` loop (which handles cell status recomputation), add a second loop over `AllTests`:
```go
for ati := range col.AllTests {
    testKey := crtest.Identification{
        RowIdentification:    col.AllTests[ati].RowIdentification,
        ColumnIdentification: col.AllTests[ati].ColumnIdentification,
    }
    if err := c.middlewares.PostAnalysis(testKey,
        &report.Rows[ri].Columns[ci].AllTests[ati].TestComparison); err != nil {
        return err
    }
}
```

Cell status recomputation stays based on `RegressedTests` only (unchanged).

### 6. Remove LinkInjector status guard
**File:** `pkg/api/componentreadiness/middleware/linkinjector/linkinjector.go` (lines 50-53)

Remove:
```go
if testStats.ReportStatus > crtest.FixedRegression {
    return nil
}
```

### 7. Restrict `AllTests` via explicit `includeAllTests` query parameter

To avoid response bloat (~1GB at top level), `AllTests` is only populated when the caller passes `includeAllTests=true` as a query parameter. The frontend sends this parameter only at Level 4 (the test variant page, `CompReadyEnvCapabilityTest.js`), which is the only level that renders `test_details` links from `all_tests`. At all other levels, `AllTests` is omitted from the response.

The `includeAllTests` parameter is:
- Parsed in `ParseComponentReportRequest` via `ParseBoolArg` (default: `false`)
- Stored in `RequestOptions.IncludeAllTests`
- Included in `GeneratorCacheKey` so requests with and without it produce distinct cache entries
- Checked by `ComponentReportGenerator.includeAllTests()`, which gates accumulation in `getNewCellStatus`, assignment in `buildReport`, and PostAnalysis middleware processing

## Verification

1. `go vet ./pkg/...` and `make lint`
2. `go test ./pkg/api/componentreadiness/...` and `go test ./pkg/apis/...`
3. Start sippy serve, hit the `/api/component_readiness` endpoint and confirm:
   - Level 1 (`?view=...`): no `all_tests` field in response
   - Level 4 without param (`?view=...&component=X&capability=Y&testId=Z`): no `all_tests` field
   - Level 4 with param (`?view=...&component=X&capability=Y&testId=Z&includeAllTests=true`): `all_tests` present with `test_details` links
   - `regressed_tests` continues to contain only regressed tests (backward compat)
   - Cell-level `status` is unchanged (still computed from regressed tests only)
