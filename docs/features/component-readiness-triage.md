# Component Readiness Triage

## Purpose

Component Readiness compares each test's pass rate against a basis release or a threshold and flags
the tests that regressed. Triage acknowledges and tracks these regressions by associating them with
known issues, separating them from regressions that still need initial investigation.

A triage record links one or more related regressions to a tracking URL, usually a Jira bug.
Component Readiness tracks a separate regression for each affected variant and suite, so one
underlying issue often surfaces as several regressions that a single triage record covers together.

## Concepts

- Regression: a tracked test whose sample pass rate dropped enough to be significant in a view
  (`TestRegression` in [`pkg/db/models/triage.go`](../../pkg/db/models/triage.go)).
- Triage record: a link from one or more regressions to a URL, carrying a short description and a
  type (`Triage` in [`pkg/db/models/triage.go`](../../pkg/db/models/triage.go)).

## What triage does and does not mean

A regression with no triage means Sippy has no triage record for it. It does not mean that no Jira
bug exists, and it does not mean that no owner is looking into it.

Linking a regression to a triage connects it to the tracking URL. It does not assign the Jira issue to
anyone, and it does not prove the regression is fixed.

## How a human triages

1. **Find untriaged regressions:** Open the regressed tests dialog from the report and use the
   `Untriaged` tab, which lists the regressions that still need a triage record.
2. **Inspect the evidence:** In the dialog, click a regression's status icon to open its test details
   page. Review the results for each variant and job run, and file a bug if needed.
3. **Link the regression:** For authorized users of the authenticated `sippy-auth` deployment, the
   test details page shows a `Triage` button that opens the triage dialog. Add the regression to an
   `Existing Triage` that already points at the bug, or create a `New Triage` with the URL,
   description, and type.
4. **Follow the fix:** Open the tracking Jira bug, linked from the `All Triage Records` index and each
   triage detail page, to judge bug progress. Judge actual CI recovery from the report status and the
   failing job runs, not from the Jira state alone.

When you triage a single regression, the dialog's `Potential Matches` tab suggests candidate triage
records to review, not confirmed causes.

## Lifecycle

1. **Detection:** Component Readiness opens a regression when a test's sample pass rate drops enough
   to be significant in a view.
2. **Linking:** An engineer links the regression to a triage record that points at a new or existing
   Jira bug.
3. **Jira sync:** A background loader imports the linked bug and, for a triage covering a single
   release, auto-resolves the triage (not the regression) once the bug reaches `ON_QA`, `Verified`,
   `Release Pending`, or `Closed`. The loader imports only recently updated tickets (`Closed` or
   `Verified` within 14 days, other statuses within 90), so a newly linked bug that Sippy has not
   already imported and was last updated before that window stays out of the bug table, and the loader
   cannot reconcile it or auto-resolve its triage
   ([`bugloader.go`](../../pkg/dataloader/bugloader/bugloader.go)).
4. **Recovery:** A regression closes when the test no longer appears as regressed in any report. Sippy
   may reuse a recently closed regression if the test regresses again within five days, but it never
   reuses a force-closed record
   ([`regressiontracker.go`](../../pkg/api/componentreadiness/regressiontracker.go)).
5. **Continuing failures:** Sippy shows `Failed Fix` when every linked triage is resolved and
   qualifying failures follow the latest resolution. Check the test details and the `Unresolved` tab
   for the ongoing failures.
6. **Reopened bug:** If the bug later leaves those states, the loader clears only resolutions it set
   automatically, never a user or rolled-off resolution.

## Statuses you will see

The regressed tests dialog tabs filter the regression list rather than a single record
([`RegressedTestsModal.jsx`](../../sippy-ng/src/component_readiness/RegressedTestsModal.jsx),
[`CompReadyUtils.jsx`](../../sippy-ng/src/component_readiness/CompReadyUtils.jsx)):

- `Unresolved`: still-failing regressions, including triaged-but-unresolved and failed-fix ones, but excluding those believed fixed.
- `Untriaged`: regressions with no triage record yet.
- `Triaged`: regressions grouped under their triage records.
- `All`: every regression in the current report.

A Component Readiness report cell shows a single aggregate status; when it contains several
regressions, that is the most severe of them. The regressed tests dialog can show each regression's
own status in its `Status` column
([`crtest/types.go`](../../pkg/apis/api/componentreport/crtest/types.go),
[`regressiontracker.go`](../../pkg/api/componentreadiness/middleware/regressiontracker/regressiontracker.go)):

- `Significant Regression` or `Extreme Regression (>15%)`: untriaged and failing.
- `Triaged Regression` or `Extreme Triaged Regression (>15%)`: linked to an unresolved triage and still failing.
- `Believed Fixed`: every triage is resolved and no qualifying failures followed the resolution time.
- `Failed Fix`: every triage is resolved, yet qualifying failures followed the resolution time.

A triage record shows its state on the `All Triage Records` index as unresolved or resolved on a date.
The failed-fix state does not appear on that index; the triage detail page and the report can show it,
because they have the regression data the index panel lacks. Key tests tolerate a few post-resolution
failures before the report flips them from `Believed Fixed` to `Failed Fix`.

## Reviewing closed regressions

A closed regression no longer appears in the current report, but triage detail pages and the regressed
tests dialog still list it so you can review the evidence that led to it. Each listed regression links
to its test details once per view it was tracked in:

| Regression | In this view | Link |
|------------|--------------|------|
| Open | Still regressed | The view's current report, shown with the severity icon. |
| Open | Closed in this view only | A historical report, labeled `Closed in this view`. |
| Closed (including force closed) | Any | A historical report, labeled `Closed regression`. |

The recorded closure takes precedence over report data. A cached report built before the closure can
still list the regression, but the row links to the historical report.

Historical rows show a clock icon with a green check badge. The tooltip names the covered period, for
example "Closed regression. View historical test details for September 22-29, 2026." The green check
means either the tracked regression closed or, for a regression that is still open elsewhere, it closed
in that view only. It does not mean the regression closed everywhere, and it is not a passing
assessment for that period. When Sippy cannot build a link (the regression has no recorded last
failure, the view is no longer configured, the view's basis cannot be resolved, or it is a spot-check
regression), the icon is disabled and its tooltip shows the reason the API returned in
`historical_link_unavailable` (see `InjectRegressionHATEOASLinks` in
`pkg/api/componentreadiness/triage.go`). Spot-check regressions have no
historical link because test details always resolve spot-check samples from the current time, so the
link would show recent runs rather than the historical period.

The stable `/component_readiness/regressions/<id>` link opens a view's current report when the
regression is still open in one, preferring the main view, and falls back to a historical report only
when every association has closed.

A historical report covers the week ending with the regression's last failure, compared against the
basis resolved from the view's current configuration (rolling basis dates are evaluated from the last
failure). Sippy does not reconstruct the view configuration that applied at that time. The test
details page keeps the computed statistical result, even when it is red, and adds a notice with the
regression ID, closure, last failure, and the sample and basis periods. The notice title follows the
closure scope:

| Case | Notice title | Closed line |
|------|--------------|-------------|
| The regression closed | `Closed regression, historical report` | The regression's closure date. |
| Closed in this view only | `Closed in this view, historical report` | The view's closure date when one was recorded, noting the regression is still open in another view. |
| Open, and the report has no view (for example after Generate Report) | `Historical report for a tracked regression` | Per-view closure is unknown because the report has no view. |
| Open, and the report's view association is still active (for example the view was edited in the URL, or the association reopened after the link was generated) | `Historical report for an open regression` | `Open`. |

Closure details are shown only after the regression record matches the report's test, release,
variants, and view. Otherwise the notice is a warning instead: the record does not match (listing the
mismatched fields), the regression was not found, or the regression could not be loaded. Viewing history never changes regression tracking or triage state. The
link holds concrete dates, so sharing or reopening it shows the same requested period; the dates do not
preserve the original view configuration.

Keep in mind:

- The report uses the view's current grouping and filters, so it can differ slightly from the report
  that originally flagged the regression.
- If the providers no longer hold data for that period, the page shows the closure details and states
  that no test results are available. It never substitutes today's data.
- The last failure is tracked per regression, not per view. For a regression that closed in one view
  but stayed open in another, the period may end after it closed in the first view.

To try this locally, `sippy seed-data` adds two closed regressions to the network triage
(`OCPBUGS-40002`), which also has an active regression, so they are reachable from the triage list.
They are in the seed sample release (the sample release of the `4.22-main` seed view) and use the
view's basis release, so they follow the seed views to a new release. Each test gets extra failing
sample release runs in a five day window that ends two days before the current sample window, and its
historical report shows it as an extreme regression against the basis.
One was tracked in both `4.22-main` and `4.22-gcp-only`, so it has two historical links. Both tests
pass in the current report, so regression tracking does not reopen them. The definitions are
`historicalRegressionSeeds` in
[`cmd/sippy/seed_data.go`](../../cmd/sippy/seed_data.go).

## Source pointers

| Path | Contents |
|------|----------|
| [`pkg/db/models/triage.go`](../../pkg/db/models/triage.go) | `Triage` and `TestRegression` models. |
| [`pkg/api/componentreadiness/regressiontracker.go`](../../pkg/api/componentreadiness/regressiontracker.go) | Opens, reuses, closes, and force closes regression records. |
| [`pkg/api/componentreadiness/middleware/regressiontracker/regressiontracker.go`](../../pkg/api/componentreadiness/middleware/regressiontracker/regressiontracker.go) | Computes the believed-fixed and failed-fix report statuses. |
| [`pkg/dataloader/bugloader/bugloader.go`](../../pkg/dataloader/bugloader/bugloader.go) | Imports linked bugs and auto-resolves or unresolves triages from Jira status. |
| [`sippy-ng/src/component_readiness/RegressedTestsModal.jsx`](../../sippy-ng/src/component_readiness/RegressedTestsModal.jsx) | Regressed tests dialog and its tabs. |
| [`sippy-ng/src/component_readiness/TriageList.jsx`](../../sippy-ng/src/component_readiness/TriageList.jsx) | `All Triage Records` index. |
| [`sippy-ng/src/component_readiness/Triage.jsx`](../../sippy-ng/src/component_readiness/Triage.jsx) | Triage detail page. |
| [`pkg/api/componentreadiness/triage.go`](../../pkg/api/componentreadiness/triage.go) | Builds current and historical `test_details:<view>` links for regressions. |
| [`sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx`](../../sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx) | Triaged regression table with historical icons and links. |
| [`sippy-ng/src/component_readiness/HistoricalRegressionNotice.jsx`](../../sippy-ng/src/component_readiness/HistoricalRegressionNotice.jsx) | Closure notice on historical test details pages. |
| [`sippy-ng/src/component_readiness/HistoricalRegressionUtils.jsx`](../../sippy-ng/src/component_readiness/HistoricalRegressionUtils.jsx) | Closure scope, table cell, link selection, and URL date helpers shared by the table, redirect, and notice. |

## API and tests

The triage and regression endpoints live under `/api/component_readiness/` and use HATEOAS links; see
[`pkg/api/componentreadiness/triage.go`](../../pkg/api/componentreadiness/triage.go) and
[`pkg/api/README.md`](../../pkg/api/README.md). The Playwright BDD suite covers these flows under
[`sippy-ng/bdd/features/`](../../sippy-ng/bdd/features/):
[`triage-index.feature`](../../sippy-ng/bdd/features/triage-index.feature),
[`triage-detail.feature`](../../sippy-ng/bdd/features/triage-detail.feature),
[`regressed-tests-dialog.feature`](../../sippy-ng/bdd/features/regressed-tests-dialog.feature),
[`regression-redirect.feature`](../../sippy-ng/bdd/features/regression-redirect.feature), and
[`historical-regressions.feature`](../../sippy-ng/bdd/features/historical-regressions.feature). Run it with
`make test-bdd`.
