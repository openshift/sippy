# Component Readiness Triage

## Purpose

Component Readiness flags tests whose pass rate regressed against a basis release. A triage record
links one or more related regressions to a tracking URL, usually a Jira bug. Triage lets the team
track a known issue without investigating it twice, and it keeps clear which regressions still need
attention. Component Readiness splits each regression by variant and suite, so one underlying issue
often surfaces as several regressions that a single triage covers together.

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
2. **Inspect the evidence:** Open a regression to read its test details for each variant and job run.
   The route `/sippy-ng/component_readiness/regressions/{id}` redirects to those details, so a link
   built from a regression id resolves to the right test
   ([`RegressionRedirect.jsx`](../../sippy-ng/src/component_readiness/RegressionRedirect.jsx)).
3. **Link the regressions:** Add them to an `Existing Triage` that already points at the bug, or
   create a `New Triage` with the URL, description, and type. `Potential Matches` suggests candidates
   to review, not confirmed causes.
4. **Follow the fix:** Open the tracking Jira bug, linked from the `All Triage Records` index and each
   triage detail page, to judge bug progress. Judge actual CI recovery from the report status and the
   failing job runs, not from the Jira state alone.

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

A report cell shows a single aggregate status; when it contains several regressions, that is the most
severe of them. The regressed tests dialog can show each regression's own status in its `Status` column
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

## API and tests

The triage and regression endpoints live under `/api/component_readiness/` and use HATEOAS links; see
[`pkg/api/componentreadiness/triage.go`](../../pkg/api/componentreadiness/triage.go) and
[`pkg/api/README.md`](../../pkg/api/README.md). The Playwright BDD suite covers these flows under
[`sippy-ng/bdd/features/`](../../sippy-ng/bdd/features/):
[`triage-index.feature`](../../sippy-ng/bdd/features/triage-index.feature),
[`triage-detail.feature`](../../sippy-ng/bdd/features/triage-detail.feature),
[`regressed-tests-dialog.feature`](../../sippy-ng/bdd/features/regressed-tests-dialog.feature), and
[`regression-redirect.feature`](../../sippy-ng/bdd/features/regression-redirect.feature). Run it with
`make test-bdd`.
