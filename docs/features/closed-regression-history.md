# Historical test details for closed regressions

Component Readiness regressions roll off the current report when they close, or
when their association with a view closes. Without help, the test results that
led to a resolved regression could only be found by rebuilding a historical
query by hand. Sippy now keeps a `test_details:<view>` link for every recorded
view association of a regression. Open associations link to the view's current
report. Closed ones link to a historical report anchored to the regression's last
failure.

This covers Component Readiness test details navigation only. Test Analysis
navigation and its run history actions are separate.

## When a link is historical

The decision is made per regression and per view, never from the triage's
resolved flag or the linked Jira status. One triage can contain both kinds.

| Regression | View association | Link |
|------------|------------------|------|
| Open | Active | Current report for the view (unchanged behavior). |
| Open | Inactive | Historical, labeled "Closed in this view". |
| Closed (including force closed) | Any | Historical, labeled "Closed regression". |

No link is produced, and the UI explains why, when the regression has no
recorded `last_failure`, when the view is no longer configured, or when the
view's basis cannot be resolved (for example a missing GA date).

## Date semantics

The sample window covers whole UTC calendar days. The basis follows the
view's configured boundaries, which may be absolute timestamps.

* **Sample**: from 00:00 UTC seven calendar days before the last-failure date,
  through the whole last-failure date. The end is serialized as the following
  UTC midnight and is exclusive, matching the data providers'
  `>= start AND < end` filters. It is an inclusive calendar-date range, not an
  exact 168 hour interval. A last failure at `2026-09-29T01:33:26Z` gives
  `sampleStartTime=2026-09-22T00:00:00Z` and
  `sampleEndTime=2026-09-30T00:00:00Z`, shown as "September 22–29, 2026". The
  end is never the exact last-failure timestamp, because that would exclude the
  failure being investigated.
* **Sample release**: the regression's persisted release.
* **Basis**: the view's basis, resolved relative to the last failure wherever it
  depends on "now". `ga-30d` to `ga` and absolute dates keep their configured
  meaning (for `5.0-main`: 4.22, `2026-05-10T00:00:00Z` to
  `2026-06-09T23:59:59Z`). A rolling basis such as `now-60d` to `now-30d` is
  evaluated from the last failure rather than from the request time. When the
  regression was detected against an earlier release, `testBasisRelease` carries
  the override as it does for current links.
* **Everything else** (grouping, include and compare variants, lifecycle and
  capability filters, confidence and other advanced options) comes from the
  view's current configuration. The link reconstructs a historical query; it is
  not a stored snapshot of the original report.

Concrete dates are serialized into the link, so sharing or reopening it never
advances the windows. Explicit RFC 3339 boundaries that have already passed are
not moved by the cache rounding applied to request times, including an exclusive
end at today's UTC midnight. Only boundaries at or after the current time are
still rounded to keep "now"-like requests cacheable.

## Data flow

1. `InjectRegressionHATEOASLinks` builds links for the regression endpoints and,
   through `RegressionLinkConfig`, for regressions embedded in
   `GET /api/component_readiness/triages` and `GET /api/component_readiness/triages/{id}`.
   Link construction uses only the regression record and the view and release
   configuration. It never runs a historical report.
2. Historical links add `regressionId=<id>` so the UI can find the regression
   the report reconstructs. The API ignores that parameter.
3. The triaged regression table (triage details and the shared table in the
   Component Readiness regressed tests dialog) builds its view columns from the
   current report data plus every view in which a listed regression is closed,
   so a closed regression keeps its column after `regressed_tests` is empty.
4. The test details page fetches `GET /api/component_readiness/regressions/<id>`
   alongside the statistical report. It checks that the record matches the
   requested test ID, release, variants, and view before showing a "Closed
   regression, historical report" notice with the regression ID, closure date,
   last failure, and the sample and basis windows. The closure metadata is
   loaded independently of the cached statistical report.

## User interface

* Open rows keep their severity icon and link.
* Closed rows show a history (clock) icon with a green check badge, linked to the
  historical report. The tooltip and accessible name read, for example, "Closed
  regression. View historical test details for September 22–29, 2026." Closure
  only in one view reads "Closed in this view" instead. The green check means
  the tracked regression closed; it is not a passing assessment for the period.
* Closed rows without a link show a disabled history icon whose tooltip explains
  why.
* The test details page keeps the computed statistical assessment, even when it
  is red, and lists the regression's triages. Viewing history never changes
  regression tracking state, and closed records are not treated as active
  regressions.
* Explicit URL parameters take precedence over view defaults in the report
  controls. An exclusive midnight end is shown as the last included day, and
  regenerating the report without editing that date keeps the original value.
  `regressionId` is kept when the report is regenerated and cleared when another
  view is selected.

## Limitations

* The historical report runs against whatever data the providers still hold.
  If the window's data has aged out, the page shows the persisted closure
  details and states that no test results are available; it never substitutes
  today's window or claims a passing result.
* The view's current configuration is used, so a view whose grouping or filters
  changed since the regression closed can produce a slightly different report
  than the one that originally flagged it.
* The anchor is the regression's last failure across all views, because
  associations do not record their own. For a view association that closed
  while the regression stayed open in another view, the window ends at that
  later failure and may not cover the period when the association was active.
* A regression record that does not match the report, or that no longer exists,
  produces a warning instead of closure details.

## Key code locations

* `pkg/api/componentreadiness/triage.go`: `InjectRegressionHATEOASLinks`,
  `HistoricalSampleWindow`, `generateHistoricalTestDetailsURL`,
  `RegressionLinkConfig`, `GetTriage`, `ListTriages`.
* `pkg/util/utils.go`: `ParseCRReleaseTimeAt` keeps explicit past boundaries.
* `pkg/sippyserver/server.go`: `regressionLinkConfig` wires views and releases
  into the triage endpoints.
* `sippy-ng/src/component_readiness/CompReadyUtils.jsx`: table column, cell,
  date range, URL precedence, and regression validation helpers.
* `sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx`: historical
  icon and links.
* `sippy-ng/src/component_readiness/TestDetailsReport.jsx` and
  `HistoricalRegressionNotice.jsx`: closure context on test details.
* `sippy-ng/src/component_readiness/CompReadyVars.jsx`: URL over view
  precedence and `regressionId` handling.
