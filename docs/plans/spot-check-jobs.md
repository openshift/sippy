# Spot-Check Jobs in Component Readiness

## Context

Component Readiness is built around statistical analysis of **test pass rates** across
many runs of multiple prow job configurations. The sample is typically the last 7 days
and the basis is the 30 days prior to the last release's GA — a very stable period.
Fisher's Exact Test (or pass-rate thresholds) determines whether a regression is
statistically significant.

We need to also monitor **rarely-run spot-check jobs** that verify obscure
configurations (vSphere hybrid installs, CPU partitioning, etcd scaling, etc.). These
jobs are run once and retried up to three times if they fail. The requirement is
simple: **the job must pass at least once in a 30-day window**. There is no basis
comparison, no Fisher's Exact, no confidence/minFail/pity thresholds — just a binary
"did it pass."

These results must appear in the existing Component Readiness report grid, be
drillable via test details, and participate in regression tracking and triage.

### Existing "Rarely Run" Concept

There is an existing `VariantJunitTableOverride` mechanism
(`pkg/apis/config/v1/types.go:30-44`) that was intended for rarely-run jobs. It pulls
junit results from alternate BigQuery tables with extended date ranges. This is not
what we want — it is still based on test pass rates within rarely-run jobs, which is
too strict. We want **full job pass**, at least once, across a few retries.

The existing `JobTier: rare` value will be replaced with `JobTier: spotcheck`.

## Design

### 1. New Job Variants

Three changes to the variant system identify spot-check jobs:

**New variant constants** in `pkg/variantregistry/ocp.go`:

```go
const (
    VariantSpotCheckComponent  = "SpotCheckComponent"
    VariantSpotCheckCapability = "SpotCheckCapability"
)
```

**New setter function** `setSpotCheckComponent` registered in `IdentifyVariants`
**before** `setJobTier`. When matched, it atomically sets all three variants:

```go
func setSpotCheckComponent(_ logrus.FieldLogger, variants map[string]string, jobName string) {
    spotCheckPatterns := []struct {
        substrings []string
        component  string
        capability string
    }{
        {[]string{"-cpu-partitioning"}, "Node", "CPU Partitioning"},
        {[]string{"-etcd-scaling"}, "etcd", "Scaling"},
        {[]string{"-vsphere-hybrid"}, "Installer", "vSphere Hybrid"},
    }

    for _, entry := range spotCheckPatterns {
        if allSubstringsMatch(jobName, entry.substrings) {
            variants[VariantSpotCheckComponent] = entry.component
            variants[VariantSpotCheckCapability] = entry.capability
            variants[VariantJobTier] = "spotcheck"
            return
        }
    }
}
```

**Remove `"rare"` patterns** from `setJobTier` (lines 743-744). Jobs that were `rare`
either become `spotcheck` (via the new setter) or stay `candidate` until promoted.

**Add to `importantVariants`** in `pkg/testidentification/ocp_variants.go` so they
flow through to the `job_variants` BigQuery table.

**JobTier lifecycle:**

```
candidate → spotcheck
              ↑
    add pattern to setSpotCheckComponent in ocp.go
```

### 2. View Configuration: Spot-Check Sample Window

Views define a separate 30-day spot-check sample window. The normal 7-day test sample
window is unsuitable because spot-check jobs run infrequently.

**New field in `pkg/apis/api/componentreport/crview/types.go`:**

```go
type View struct {
    // ... existing fields ...
    SpotCheckSample *reqopts.RelativeRelease `json:"spot_check_sample,omitempty" yaml:"spot_check_sample,omitempty"`
}
```

**Example in `config/views.yaml`:**

```yaml
- name: 5.0-main
  sample_release:
    release: "5.0"
    relative_start: now-7d
    relative_end: now
  spot_check_sample:
    release: "5.0"
    relative_start: now-30d
    relative_end: now
```

**Carry resolved times through to `pkg/apis/api/componentreport/reqopts/types.go`:**

```go
type RequestOptions struct {
    // ... existing fields ...
    SpotCheckSample *Release `json:"spot_check_sample,omitempty"`
}
```

**Resolve in `pkg/api/componentreadiness/utils/queryparamparser.go`:**
In `ParseComponentReportRequest`, after resolving view defaults (~line 50):

```go
if view != nil && view.SpotCheckSample != nil {
    resolved, err := GetViewReleaseOptions(releases, "spot_check", *view.SpotCheckSample, crTimeRoundingFactor)
    if err != nil {
        return
    }
    opts.SpotCheckSample = &resolved
}
```

Views **do not** add `"spotcheck"` to their `JobTier` include list. Normal test
queries filter on `JobTier: [blocking, informing, standard]`, which naturally excludes
spot-check jobs. The spot-check middleware independently queries with its own window.

### 3. New Middleware: `spotcheckjobs`

**Location:** `pkg/api/componentreadiness/middleware/spotcheckjobs/spotcheckjobs.go`

Implements `middleware.Middleware` (interface at
`pkg/api/componentreadiness/middleware/interface.go:15-40`).

```go
type SpotCheckJobs struct {
    dataProvider dataprovider.DataProvider
    reqOptions   reqopts.RequestOptions
    log          logrus.FieldLogger

    // Populated during QueryTestDetails, consumed by PreTestDetailsAnalysis
    sampleJobDetails map[string][]dataprovider.JobRunDetail
}
```

#### Query Phase — Inject Synthetic Test Results

Queries the BQ `jobs` table (joined with `job_variants`) for all jobs with
`JobTier = 'spotcheck'` in the spot-check sample window. Applies normal view variant
filters (Platform, Architecture, etc.) except JobTier. Groups results by
(SpotCheckComponent, SpotCheckCapability, column variant combo).

Each group becomes one synthetic test result injected via `sampleStatusCh`:

- **At least one pass:** `TestStatus{TotalCount: 1, SuccessCount: 1}` → green
- **No passes:** `TestStatus{TotalCount: 1, SuccessCount: 0}` → red

No basis data is injected. The `Component` and `Capabilities` fields on `TestStatus`
come from the `SpotCheckComponent` / `SpotCheckCapability` variant values, placing the
synthetic test in the correct row of the CR grid.

The synthetic test ID is deterministic:
`spotcheck:<component-lower>:<capability-kebab-lower>`

The synthetic test name:
`[spot-check] <Component> / <Capability> must pass at least once per sample window`

#### PreAnalysis Phase — Bypass Fisher's, Set Status Directly

```go
func (s *SpotCheckJobs) PreAnalysis(testKey crtest.Identification,
    testStats *testdetails.TestComparison) error {

    if !strings.HasPrefix(testKey.TestID, "spotcheck:") {
        return nil
    }

    if testStats.SampleStats.SuccessCount > 0 {
        testStats.ReportStatus = crtest.NotSignificant
        testStats.Explanations = append(testStats.Explanations,
            "Spot-check job passed at least once in the sample window")
    } else {
        testStats.ReportStatus = crtest.ExtremeRegression
        testStats.Explanations = append(testStats.Explanations,
            fmt.Sprintf("Spot-check job did not pass in the %d-day sample window", ...))
    }

    testStats.Comparison = crtest.SpotCheck
    testStats.AnalysisComplete = true  // signals assessComponentStatus to skip
    return nil
}
```

This completely bypasses Fisher's Exact, confidence thresholds, minFail, pity factor,
and basis comparison. The middleware directly determines the outcome.

#### QueryTestDetails Phase — Load Job Run Details

Queries individual job runs (not aggregated) for spot-check jobs, storing them for
`PreTestDetailsAnalysis` to inject as synthetic `TestJobRunRows`.

#### PreTestDetailsAnalysis Phase — Inject Job Runs for Drill-Down

Injects the cached job run rows into `status.SampleStatus` so the test details page
renders individual job runs with pass/fail and prow links.

#### PostAnalysis Phase — No-op

The `RegressionTracker` middleware handles regression injection. The
`RegressionAllowances` middleware should skip spot-check tests (they have no basis).

### 4. AnalysisComplete Flag

**New field in `pkg/apis/api/componentreport/testdetails/types.go`:**

```go
type TestComparison struct {
    // ... existing fields ...
    AnalysisComplete bool `json:"-"`
}
```

**Guard in `pkg/api/componentreadiness/component_report.go`:**

```go
func (c *ComponentReportGenerator) assessComponentStatus(
    testStats *testdetails.TestComparison, logger *log.Entry) {
    if testStats.AnalysisComplete {
        return
    }
    // ... existing Fisher's / pass-rate logic unchanged ...
}
```

This is a minimal change that allows any middleware to fully control analysis outcome.

### 5. New Comparison Type

**New constant in `pkg/apis/api/componentreport/crtest/types.go`:**

```go
const (
    PassRate    Comparison = "pass_rate"
    FisherExact Comparison = "fisher_exact"
    SpotCheck   Comparison = "spot_check"    // new
)
```

### 6. New BigQuery Queries

**New methods on `DataProvider` interface
(`pkg/api/componentreadiness/dataprovider/interface.go`):**

```go
type SpotCheckQuerier interface {
    QuerySpotCheckJobRuns(ctx context.Context, reqOptions reqopts.RequestOptions,
        allJobVariants crtest.JobVariants,
        start, end time.Time) ([]SpotCheckGroup, error)

    QuerySpotCheckJobRunDetails(ctx context.Context, reqOptions reqopts.RequestOptions,
        allJobVariants crtest.JobVariants,
        spotCheckComponent, spotCheckCapability string,
        variants map[string]string,
        start, end time.Time) ([]JobRunDetail, error)
}

type SpotCheckGroup struct {
    SpotCheckComponent  string
    SpotCheckCapability string
    Variants            map[string]string
    TotalRuns           int
    SuccessfulRuns      int
    JobNames            []string
}

type JobRunDetail struct {
    JobName   string
    RunID     string
    URL       string
    StartTime time.Time
    Success   bool
}
```

**Component report query** (`QuerySpotCheckJobRuns`):

```sql
SELECT
    jv_SpotCheckComponent.variant_value AS spot_check_component,
    jv_SpotCheckCapability.variant_value AS spot_check_capability,
    -- dynamic column group by variants (Platform, Arch, Network, etc.)
    COUNT(DISTINCT jobs.prowjob_build_id) AS total_runs,
    COUNT(DISTINCT IF(jobs.prowjob_state = 'success',
        jobs.prowjob_build_id, NULL)) AS successful_runs,
    ARRAY_AGG(DISTINCT jobs.prowjob_job_name) AS job_names
FROM {dataset}.jobs jobs
    LEFT JOIN {dataset}.job_variants jv_SpotCheckComponent
        ON jobs.prowjob_job_name = jv_SpotCheckComponent.job_name
        AND jv_SpotCheckComponent.variant_name = 'SpotCheckComponent'
    LEFT JOIN {dataset}.job_variants jv_SpotCheckCapability
        ON jobs.prowjob_job_name = jv_SpotCheckCapability.job_name
        AND jv_SpotCheckCapability.variant_name = 'SpotCheckCapability'
    LEFT JOIN {dataset}.job_variants jv_Release ...
    -- other variant joins for column group by
WHERE jobs.prowjob_start >= @From
    AND jobs.prowjob_start < @To
    AND jv_Release.variant_value = @Release
    AND jv_SpotCheckComponent.variant_value IS NOT NULL
    -- view include_variant filters (Platform, Architecture, etc.)
GROUP BY spot_check_component, spot_check_capability, variant_Platform, ...
```

Hits only the `jobs` table (joined with `job_variants`), never `junit`. Fast and cheap.

**Test details query** (`QuerySpotCheckJobRunDetails`): Returns individual job runs
with pass/fail for a specific component/capability/variant combo.

### 7. Middleware Registration

In `pkg/api/componentreadiness/component_report.go`, `initializeMiddleware()`:

```go
func (c *ComponentReportGenerator) initializeMiddleware() {
    c.middlewares = middleware.List{}

    if c.ReqOptions.SpotCheckSample != nil {
        c.middlewares = append(c.middlewares,
            spotcheckjobs.NewSpotCheckJobsMiddleware(c.dataProvider, c.ReqOptions))
    }

    // ... existing middleware (releasefallback, regressiontracker, regressionallowances, linkinjector) ...
}
```

Spot-check middleware runs first so its synthetic results are in place before other
middleware processes them.

### 8. Regression Tracking

Spot-check regressions participate in the existing regression tracking pipeline
(`pkg/api/componentreadiness/regressiontracker.go:271-404`,
`SyncRegressionsForReport`). This works automatically because:

- Synthetic spot-check tests appear in `report.Rows[].Columns[].RegressedTests`
  like any other regressed test.
- `SyncRegressionsForReport` iterates all regressed tests and calls
  `backend.OpenRegression(view, regTest)` for new ones.
- The synthetic test ID (`spotcheck:installer:vsphere-hybrid`) is stable and
  deterministic, so regression records persist correctly across report runs.
- The `RegressionTracker` middleware (`PostAnalysis`) applies triage status
  (triaged, fixed, failed-fixed) to spot-check regressions the same as normal ones.

No changes needed to the regression tracking code.

### 9. Frontend: `spot_check` Comparison Type

**`sippy-ng/src/component_readiness/CompReadyTestDetailRow.js`:**

The test details row currently renders pass_rate/successes/failures/flakes in the
`infoCell` function (lines 54-67). For `comparison === "spot_check"`, render a simpler
view:

- Total job runs attempted
- Successful runs
- Required passes (1)
- No Fisher's Exact column
- No basis stats column

**`sippy-ng/src/component_readiness/TestDetailsReport.js`:**

When `data.analyses[0].comparison === "spot_check"`:
- Hide Fisher's Exact confidence display
- Hide basis release stats section
- Show explanation text prominently ("Spot-check job did not pass in 30-day window")
- Job run table still renders normally with prow links

This is a small conditional — the page structure stays the same.

## Files Changed

| File | Change |
|------|--------|
| `pkg/variantregistry/ocp.go` | Add `setSpotCheckComponent` setter, new constants, remove `"rare"` patterns from `setJobTier`, register setter before `setJobTier` |
| `pkg/testidentification/ocp_variants.go` | Add `SpotCheckComponent`, `SpotCheckCapability` to `importantVariants` |
| `pkg/apis/api/componentreport/crview/types.go` | Add `SpotCheckSample *RelativeRelease` to `View` |
| `pkg/apis/api/componentreport/reqopts/types.go` | Add `SpotCheckSample *Release` to `RequestOptions` |
| `pkg/apis/api/componentreport/testdetails/types.go` | Add `AnalysisComplete bool` to `TestComparison` |
| `pkg/apis/api/componentreport/crtest/types.go` | Add `SpotCheck` comparison constant |
| `pkg/api/componentreadiness/dataprovider/interface.go` | Add `SpotCheckQuerier` interface, `SpotCheckGroup`, `JobRunDetail` types |
| `pkg/api/componentreadiness/dataprovider/bigquery/provider.go` | Implement `QuerySpotCheckJobRuns`, `QuerySpotCheckJobRunDetails` |
| `pkg/api/componentreadiness/middleware/spotcheckjobs/spotcheckjobs.go` | **New file** — middleware implementation |
| `pkg/api/componentreadiness/component_report.go` | Add `AnalysisComplete` guard in `assessComponentStatus`, register middleware in `initializeMiddleware` |
| `pkg/api/componentreadiness/utils/queryparamparser.go` | Resolve `SpotCheckSample` from view config |
| `config/views.yaml` | Add `spot_check_sample` to relevant views |
| `sippy-ng/src/component_readiness/CompReadyTestDetailRow.js` | Conditional rendering for `spot_check` comparison |
| `sippy-ng/src/component_readiness/TestDetailsReport.js` | Hide Fisher/basis sections for `spot_check` |

## What Doesn't Change

- Fisher's Exact / pass-rate analysis logic (guarded by `AnalysisComplete`)
- Report types (`ComponentReport`, `ReportRow`, `ReportColumn`, `ReportTestSummary`)
- URL param structure (`testId`, `component`, `capability`, variant params all work)
- Caching strategy
- Regression tracking code (spot-check tests participate automatically)

## Verification

1. **Variant mapping:** After adding patterns to `setSpotCheckComponent`, run the
   variant loader and verify jobs get `SpotCheckComponent`, `SpotCheckCapability`,
   and `JobTier: spotcheck` variants in BigQuery's `job_variants` table.

2. **Component report:** Load a view with `spot_check_sample` configured. Verify
   spot-check tests appear in the correct component/capability rows. Break a
   spot-check job (or test with a job that has no passes in the window) and verify
   it shows as an extreme regression.

3. **Test details drill-down:** Click a spot-check regression in the CR grid. Verify
   the test details page shows individual job runs with prow links, no Fisher's
   Exact stats, and the spot-check explanation text.

4. **Regression tracking:** Verify a failing spot-check job creates a
   `test_regressions` record. Verify triaging the regression via the normal triage
   flow works (status changes to triaged/fixed/failed-fixed).

5. **No interference:** Verify normal test-based regressions are unaffected —
   Fisher's Exact still runs, basis comparison still works, existing views show the
   same results.
