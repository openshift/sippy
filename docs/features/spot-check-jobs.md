# Spot-check jobs

Spot-check jobs let Component Readiness show signal from jobs that run too
rarely, or are too specialized, for the normal statistical comparison. Examples
are a job for one install configuration or one stable feature that runs about
once a month. Each such job is owned by a component and a capability, and the
only requirement is that it passes at least once in a sample window. A job that
keeps failing turns that component and capability red.

Spot-check analysis is implemented for PostgreSQL only. The BigQuery data
provider returns no spot-check results, so views served from BigQuery simply
show no spot-check rows.

## Onboarding a job

A job takes part when its variants say so. Add an entry to
`componentCapabilityPatterns` in `pkg/variantregistry/ocp.go` with a substring
of the job name, a real OCPBUGS component, a capability, and the job tier
`spotcheck-30d`. The registry then sets three variants on the job: `Component`,
`Capability` and `JobTier`. A job with a `spotcheck-` tier and no component or
capability fails variant registry validation. Run `make update-variants` after
changing the patterns so the variants snapshot stays current.

Retries are not done by Sippy. The intended setup is that something outside
Sippy re-runs a failed spot-check job a few times over a few days, so a single
failure is treated as pending rather than as a regression.

## Enabling it for a view

Views opt in with `spot_check_job_samples` in `config/views.yaml`, keyed by a
sample name:

```yaml
spot_check_job_samples:
  spotcheck-30d:
    relative_start: now-30d
    relative_end: now
    include_variants:
      JobTier:
        - spotcheck-30d
```

The sample window is independent of the view's ordinary sample window (7 days
in the example view), and the release is inherited from the view's sample
release. `include_variants` selects which jobs belong to the sample. A view
with no `spot_check_job_samples` has spot-check disabled, both in the API and
in the background regression cache loader.

## How results are produced

1. At ingestion, the OpenShift synthetic test manager creates the synthetic test
   `[sig-sippy] openshift-tests should work` for every job run
   (`pkg/synthetictests/ocp_synthetic_tests.go`). It passes when the job passed
   overall, including when only informing tests failed. It fails when the job
   failed with test failures. Aborted and errored runs that never established
   success produce no pass, so an interrupted retry cannot clear real failures.
2. When a report is generated, `ComponentReportGenerator.getTestStatus` runs one
   `QuerySpotCheckTestStatus` query per configured sample, concurrently with the
   normal status query. The PostgreSQL implementation counts runs of the
   synthetic test in the sample window for jobs matching `include_variants`,
   and groups them by `Component`, `Capability` and the `db_group_by` variants.
   Requested capability and exact test filters are applied to these results,
   because they are merged in after the normal queries have already filtered.
3. Each group gets a synthetic test ID of the form
   `spotcheck-30d:etcd:scaling` (sample name, lowercase component, capability
   with spaces replaced by dashes). The format lives in
   `pkg/apis/api/componentreport/crtest/spotcheck.go`. The encoding is lossy, so
   capability names that contain hyphens are not supported.
4. The results are merged into the sample status, and the `spotcheckjobs`
   middleware, which runs before the other analysis middleware, claims them. It
   has no base comparison and decides the status from run counts alone:

   | Runs in the sample window | Status |
   | --- | --- |
   | At least one pass | `NotSignificant` |
   | One failure, no passes | `MissingSample` (awaiting retry) |
   | Two failures, no passes | `SignificantRegression` |
   | Three or more failures, no passes | `ExtremeRegression` |
   | No runs | `MissingSample` |

   The comparison is reported as `spot_check`, and the sample statistics carry
   the spot-check window rather than the ordinary sample window.

## Drill-down

Test details for a spot-check test ID use `QuerySpotCheckTestDetails` to return
the individual job runs. The window comes from the view's configured sample. A
request without a view, or for a view with no configured samples, falls back to
its own sample dates, but only when it asks for a spot-check test. Ordinary
custom reports never include spot-check results.

## Caching

The spot-check samples are part of the report cache key (`GeneratorCacheKey`),
so different windows, tier filters, or a view with spot-check disabled never
share cached results. The background regression cache loader builds its
requests from the view's configured samples, matching the API.

## Frontend

The test details page detects `spot_check` comparisons and drops the basis
side. `CompReadyTestPanel.jsx` labels the sample dates as the spot-check window,
`CompReadyTestDetailRow.jsx` omits the basis columns, and `TestDetailsReport.jsx`
skips the base override notice.

## Key code locations

| Path | Role |
| --- | --- |
| `pkg/variantregistry/ocp.go` | Job to component, capability and tier mapping, and validation. |
| `pkg/apis/api/componentreport/crview/types.go` | `spot_check_job_samples` view configuration. |
| `pkg/api/componentreadiness/utils/queryparamparser.go` | Resolves samples from the view, and the drill-down fallback. |
| `pkg/synthetictests/ocp_synthetic_tests.go` | The synthetic test whose passes and failures drive the analysis. |
| `pkg/api/componentreadiness/dataprovider/postgres/provider.go` | `QuerySpotCheckTestStatus` and `QuerySpotCheckTestDetails`. |
| `pkg/api/componentreadiness/middleware/spotcheckjobs/` | Status thresholds and reported window. |
| `pkg/apis/api/componentreport/crtest/spotcheck.go` | Synthetic test ID encoding and parsing. |
| `pkg/api/componentreadiness/component_report.go`, `test_details.go` | Concurrent queries, merging, and the cache key. |
| `pkg/dataloader/regressioncacheloader/regressioncacheloader.go` | Background loader that builds requests from view samples. |
