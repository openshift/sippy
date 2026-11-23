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

Linking a regression to a triage records that connection and nothing more. It does not assign the
Jira issue to anyone, and it does not prove the regression is fixed.

## How a human triages

1. **Find untriaged regressions:** Open the regressed tests dialog from the report and use the
   `Untriaged` tab, which lists the regressions that still need a triage record.
2. **Inspect the evidence:** Open a regression to read its test details for each variant and job run.
   A stable [regression link](../../sippy-ng/src/component_readiness/RegressionRedirect.jsx) opens
   that page, so a link shared in a bug points at the right test.
3. **Link the regressions:** Add them to an `Existing Triage` that already points at the bug, or
   create a `New Triage` with the URL, description, and type. `Potential Matches` suggests candidates
   to review, not confirmed causes.
4. **Follow the fix:** The `All Triage Records` index and each triage detail page link to the tracking
   Jira bug. Open that bug to follow the fix's progress, which the triage's own unresolved or resolved
   state does not report.

## Source pointers

| Path | Contents |
|------|----------|
| [`pkg/db/models/triage.go`](../../pkg/db/models/triage.go) | `Triage` and `TestRegression` models. |
| [`pkg/api/componentreadiness/triage.go`](../../pkg/api/componentreadiness/triage.go) | Triage API, validation, and regression linking. |
| [`pkg/sippyserver/server.go`](../../pkg/sippyserver/server.go) | Registers the triage and regression API routes. |
| [`sippy-ng/src/component_readiness/RegressedTestsModal.jsx`](../../sippy-ng/src/component_readiness/RegressedTestsModal.jsx) | Regressed tests dialog and the `Untriaged` tab. |
| [`sippy-ng/src/component_readiness/AddRegressionPanel.jsx`](../../sippy-ng/src/component_readiness/AddRegressionPanel.jsx) | Existing and new triage panel. |
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
