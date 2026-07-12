# TRT-2782 Phase 2: Frontend, use HATEOAS links for all cells

## Context

Phase 1 (complete) added an `all_tests` field to every cell in the component_readiness API response. Every entry in `all_tests` has a `test_details` HATEOAS link regardless of status. Phase 2 uses these links in the frontend so all grid cells are navigable, then removes the now-unnecessary fallback link generation and `filterVals` prop threading.

Currently, only Level 4 cells (`CompReadyTestCell`) gate clickability on regression status. Levels 1-3 are already fully clickable. The problem: `CompCapTestRow` passes `columnVal.regressed_tests?.[0] || null` to `CompReadyTestCell`, so non-regressed cells render as unclickable icons.

## Changes

### 1. Make Level 4 cells use `all_tests` instead of `regressed_tests`
**File:** `sippy-ng/src/component_readiness/CompCapTestRow.js`

Change line 46 from `regressed_tests?.[0]` to `all_tests?.[0]` with fallback to `regressed_tests?.[0]` for backward compatibility. Remove `filterVals` prop from the `CompReadyTestCell` call and from this component's props/PropTypes.

### 2. Update `CompReadyTestCell` to use simplified link generation
**File:** `sippy-ng/src/component_readiness/CompReadyTestCell.js`

- Rename prop `regressedTest` to `test`
- Remove `filterVals` prop
- Remove `console.log` debug leftover (line 36)
- Change link generation to `generateTestDetailsReportLink(test)`
- Update PropTypes

### 3. Simplify `generateTestDetailsReportLink`
**File:** `sippy-ng/src/component_readiness/CompReadyUtils.js` (lines 834-868)

Remove `filterVals` and `expandEnvironment` parameters. Remove the hand-built fallback URL. The simplified function just uses HATEOAS links via `getTestDetailsLink` + `convertApiUrlToUiUrl`, returning `null` if absent.

Also delete the unused `generateTestReport` function (lines 764-797), which has zero callers.

### 4. Update all callers of `generateTestDetailsReportLink`

- **`ComponentReadinessToolBar.js`** (line 130): Remove `filterVals` and `expandEnvironment` args. Remove `filterVals` from props, useEffect deps, PropTypes. Stop passing `filterVals` to `RegressedTestsModal`.
- **`RegressedTestsPanel.js`** (line 299): Remove `filterVals` and `expandEnvironment` args. Remove `filterVals` from props and PropTypes.
- **`TriagePotentialMatches.js`** (line 361): Remove local `filterVals` variable and `expandEnvironment` arg. Keep `viewToUse` as the `viewName` parameter.
- **`TriagedRegressionTestList.js`** (line 255): Remove `props.filterVals` and `expandEnvironment` args. Keep `viewName` parameter. Remove `filterVals` from PropTypes.

### 5. Remove `filterVals` from intermediate components that only threaded it

- **`RegressedTestsModal.js`**: Remove `filterVals` from props, from the three `RegressedTestsPanel` instances, from `TriagedTestsPanel`, and from PropTypes.
- **`TriagedTestsPanel.js`**: Remove `filterVals` from `TriagedRegressionTestList` call and PropTypes.
- **`CompReadyEnvCapabilityTest.js`**: Stop passing `filterVals` to `CompCapTestRow` and `ComponentReadinessToolBar`. Keep `filterVals` in this component (still used for API call construction at line 115).

### Summary of `filterVals` removal chain

Removing `filterVals` from `generateTestDetailsReportLink` cascades through:
- `CompReadyTestCell` -> `CompCapTestRow` -> `CompReadyEnvCapabilityTest` (row prop only)
- `RegressedTestsPanel` -> `RegressedTestsModal` -> `ComponentReadinessToolBar`
- `TriagedRegressionTestList` -> `TriagedTestsPanel` -> `RegressedTestsModal`
- `TriagePotentialMatches` (local variable, no prop)

`filterVals` continues to exist in parent page components where it is used for API call construction and Level 1-3 drill-down navigation URLs.

## Verification

1. `cd sippy-ng && npx eslint . --fix && npx prettier --write .`
2. `cd sippy-ng && npm test`
3. Start sippy serve + sippy-ng dev server
4. Navigate to Level 4 cells and verify green/grey cells are now clickable
5. Verify red cells still navigate correctly
6. Verify regressed tests modal and triage view links still work
