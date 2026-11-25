/********************************************************
 * Historical (closed regression) test details helpers
 *
 * Closed regressions roll off the current report, so the API links them to a
 * historical test details report anchored to their last failure. These helpers
 * decide how the triaged regression table, the regression redirect, and the
 * test details page present those links.
 ******************************************************** */
import {
  compareViewNames,
  convertApiUrlToUiUrl,
  dateEndFormat,
  formatLongDate,
  generateTestDetailsReportLink,
  getTestDetailsLink,
  parseVariantPairs,
  TEST_DETAILS_LINK_PREFIX,
} from './CompReadyUtils'
import { formatInTimeZone } from 'date-fns-tz'

// ClosureScope describes how a regression relates to the view a report is
// shown for. Shared by the triaged regression table and the test details
// notice so both describe a closure the same way.
export const ClosureScope = Object.freeze({
  // The tracked regression itself closed.
  REGRESSION: 'regression',
  // Only this view's association closed; the regression is open elsewhere.
  VIEW: 'view',
  // Open in this view.
  OPEN: 'open',
  // Open overall, but the report has no view, so per-view closure is unknown.
  UNKNOWN: 'unknown',
})

// LookupStatus tracks loading the regression record a historical link names.
export const LookupStatus = Object.freeze({
  IDLE: 'idle',
  LOADING: 'loading',
  LOADED: 'loaded',
  MISSING: 'missing',
  ERROR: 'error',
})

// CellKind is what the triaged regression table shows in a view column.
export const CellKind = Object.freeze({
  CURRENT: 'current',
  HISTORICAL: 'historical',
  UNAVAILABLE: 'unavailable',
})

function testDetailsLinkKey(viewName) {
  return `${TEST_DETAILS_LINK_PREFIX}${viewName}`
}

// historicalClosure is the single rule for whether a regression's association
// with a view is closed. It returns { scope, association } with scope
// ClosureScope.REGRESSION when the tracked regression closed, or
// ClosureScope.VIEW when only this view's association closed while the
// regression stays open elsewhere. It returns null when the regression is open
// in this view or was never tracked in it. The API only emits test_details
// links for recorded associations, so the association list is authoritative.
export function historicalClosure(regression, viewName) {
  const association = (regression.views || []).find(
    (v) => v.view_name === viewName
  )
  if (!association) return null
  if (regression.closed?.Valid) {
    return { scope: ClosureScope.REGRESSION, association }
  }
  if (!association.active) return { scope: ClosureScope.VIEW, association }
  return null
}

// triagedRegressionViewNames lists the status columns for the triaged
// regression table: views with current report data, plus views in which a
// listed regression is closed, so closed regressions keep their historical
// link after rolling off the current report.
export function triagedRegressionViewNames(regressedTestsByView, regressions) {
  const names = new Set(Object.keys(regressedTestsByView || {}))
  for (const regression of regressions || []) {
    for (const { view_name: viewName } of regression.views || []) {
      if (viewName && historicalClosure(regression, viewName)) {
        names.add(viewName)
      }
    }
  }
  return [...names].sort(compareViewNames)
}

// preferredTestDetailsLink picks the test_details link to follow when a
// regression is opened without a view: links to a view's current report win
// over historical ones, and main views win within each group. An open
// regression whose main association closed therefore still opens a live
// report from another view.
export function preferredTestDetailsLink(regression) {
  const links = regression?.links
  if (!links) return null
  const viewNames = Object.keys(links)
    .filter((key) => key.startsWith(TEST_DETAILS_LINK_PREFIX))
    .map((key) => key.slice(TEST_DETAILS_LINK_PREFIX.length))
    .sort(compareViewNames)
  const current = viewNames.find((name) => !historicalClosure(regression, name))
  const selected = current ?? viewNames[0]
  if (selected !== undefined) return links[testDetailsLinkKey(selected)]
  return getTestDetailsLink(links)
}

// parseUrlDate reads a date from a report URL parameter or link. Values
// without a zone, such as the controls' "yyyy-MM-dd HH:mm:ss", are UTC,
// following formatLongDate's convention. Returns null for missing or invalid
// values.
export function parseUrlDate(value) {
  if (value instanceof Date) return isNaN(value.getTime()) ? null : value
  if (!value) return null
  let text = String(value).trim().replace(' ', 'T')
  if (!/(Z|[+-]\d{2}:?\d{2})$/i.test(text)) text += 'Z'
  const date = new Date(text)
  return isNaN(date.getTime()) ? null : date
}

// lastIncludedInstant converts an exclusive end into the last instant it
// includes, so formatting it in UTC gives the last included calendar day.
// Temporal.PlainDate would express this as a date subtraction, but Temporal is
// not available in the Node runtime the helper tests run under, and the
// project has no polyfill.
function lastIncludedInstant(endExclusive) {
  return new Date(endExclusive.getTime() - 1)
}

// historicalWindowFromLink reads the concrete sample window serialized in a
// historical test_details link. The end is exclusive. The query parameter
// layout is part of the API contract documented in pkg/api/README.md.
export function historicalWindowFromLink(link) {
  if (!link) return null
  let params
  try {
    params = new URL(link, 'http://placeholder').searchParams
  } catch {
    return null
  }
  const start = parseUrlDate(params.get('sampleStartTime'))
  const endExclusive = parseUrlDate(params.get('sampleEndTime'))
  if (!start || !endExclusive) return null
  return { start, endExclusive }
}

// formatHistoricalDateRange renders an inclusive UTC calendar-date range such
// as "September 22–29, 2026" from a start and an exclusive end.
export function formatHistoricalDateRange(start, endExclusive) {
  const lastIncluded = lastIncludedInstant(endExclusive)
  const fmt = (date, pattern) => formatInTimeZone(date, 'UTC', pattern)
  const sameYear = fmt(start, 'yyyy') === fmt(lastIncluded, 'yyyy')
  const sameMonth = sameYear && fmt(start, 'MM') === fmt(lastIncluded, 'MM')
  if (fmt(start, 'yyyy-MM-dd') >= fmt(lastIncluded, 'yyyy-MM-dd')) {
    return fmt(start, 'MMMM d, yyyy')
  }
  if (sameMonth) {
    return `${fmt(start, 'MMMM d')}–${fmt(lastIncluded, 'd, yyyy')}`
  }
  if (sameYear) {
    return `${fmt(start, 'MMMM d')} – ${fmt(lastIncluded, 'MMMM d, yyyy')}`
  }
  return `${fmt(start, 'MMMM d, yyyy')} – ${fmt(lastIncluded, 'MMMM d, yyyy')}`
}

// formatUrlDateRange renders a start and end time taken from report URL
// parameters as an inclusive date range. Returns null when either value is
// missing or invalid.
export function formatUrlDateRange(startValue, endValue) {
  const start = parseUrlDate(startValue)
  const end = parseUrlDate(endValue)
  if (!start || !end) return null
  return formatHistoricalDateRange(start, end)
}

const closureLabels = {
  [ClosureScope.REGRESSION]: 'Closed regression',
  [ClosureScope.VIEW]: 'Closed in this view',
}

// unavailableReason explains why a closed regression has no usable historical
// link. The API reports why it left a link out in historical_link_unavailable
// (see InjectRegressionHATEOASLinks in pkg/api/componentreadiness/triage.go),
// so the UI never re-derives those reasons; only a link the UI cannot read is
// explained here.
function unavailableReason(regression, viewName, link) {
  if (link) return 'its link has no readable sample window'
  return (
    regression.historical_link_unavailable?.[viewName] ||
    'the API did not provide a historical link'
  )
}

// triagedRegressionViewCell decides what the triaged regression table shows
// for one regression in one view column:
//   - CellKind.CURRENT: the regression is in this view's current report; keep
//     its severity icon and link.
//   - CellKind.HISTORICAL: the regression (or this view's association) is
//     closed and the API supplied a historical test_details link.
//   - CellKind.UNAVAILABLE: closed, but no usable historical link exists.
//   - null: nothing to show.
export function triagedRegressionViewCell(
  regression,
  viewName,
  regressedTestsByView
) {
  const current = (regressedTestsByView?.[viewName] || []).find(
    (t) => t?.regression?.id === regression.id
  )
  if (current) {
    return {
      kind: CellKind.CURRENT,
      status: current.status,
      explanations: current.explanations,
      url: generateTestDetailsReportLink(current, viewName),
    }
  }

  const closure = historicalClosure(regression, viewName)
  if (!closure) return null
  const { scope } = closure
  const label = closureLabels[scope]

  const link = regression.links?.[testDetailsLinkKey(viewName)]
  const sampleWindow = historicalWindowFromLink(link)
  if (sampleWindow) {
    return {
      kind: CellKind.HISTORICAL,
      scope,
      url: convertApiUrlToUiUrl(link),
      sampleWindow,
      tooltip: `${label}. View historical test details for ${formatHistoricalDateRange(
        sampleWindow.start,
        sampleWindow.endExclusive
      )}.`,
    }
  }

  return {
    kind: CellKind.UNAVAILABLE,
    scope,
    tooltip: `${label}. Historical test details are unavailable because ${unavailableReason(
      regression,
      viewName,
      link
    )}.`,
  }
}

// triagedRegressionCellRank orders view status cells for sorting: current
// report statuses first (most severe, i.e. lowest, first), then historical
// links, then unavailable history, then empty cells.
export function triagedRegressionCellRank(cell) {
  if (cell == null) return Number.MAX_SAFE_INTEGER
  if (cell.kind === CellKind.CURRENT) return cell.status ?? 0
  if (cell.kind === CellKind.HISTORICAL) return Number.MAX_SAFE_INTEGER - 2
  return Number.MAX_SAFE_INTEGER - 1
}

// The UI date controls treat end dates as inclusive (23:59:59). Historical
// links use an exclusive end at the following UTC midnight, so those are shown
// as the last included day.
function isExclusiveMidnight(value) {
  const date = parseUrlDate(value)
  return (
    typeof value === 'string' &&
    date !== null &&
    formatInTimeZone(date, 'UTC', 'HH:mm:ss.SSS') === '00:00:00.000'
  )
}

// endTimeControlValue converts an end time from the URL into the inclusive
// value the date controls display.
export function endTimeControlValue(urlValue) {
  if (!isExclusiveMidnight(urlValue)) return urlValue
  return formatLongDate(
    lastIncludedInstant(parseUrlDate(urlValue)),
    dateEndFormat
  )
}

// endTimeParam formats an end-time control for the URL when a report is
// generated. If the control still shows the day an exclusive URL end
// represents, the original URL value is kept so the regenerated report covers
// exactly the same window.
export function endTimeParam(controlValue, urlValue) {
  const formatted = formatLongDate(controlValue, dateEndFormat)
  if (
    isExclusiveMidnight(urlValue) &&
    endTimeControlValue(urlValue) === formatted
  ) {
    return urlValue
  }
  return formatted
}

// validateHistoricalRegression checks that a regression record loaded from the
// API identifies the report being shown before its closure metadata is
// presented as this report's context. It returns the mismatched fields, the
// ClosureScope for the requested view, and the closed view association (for
// ClosureScope.VIEW, its closure date). Without a view, an open regression's
// per-view closure cannot be checked, so its scope is ClosureScope.UNKNOWN
// rather than ClosureScope.OPEN.
export function validateHistoricalRegression(regression, request) {
  if (!regression) {
    return {
      valid: false,
      mismatches: ['regression'],
      scope: null,
      association: null,
    }
  }
  const mismatches = []
  if (regression.test_id !== request.testId) mismatches.push('test ID')
  if (request.sampleRelease && regression.release !== request.sampleRelease) {
    mismatches.push('release')
  }
  const requestedVariants = request.variants || {}
  const regressionVariants = Object.entries(
    parseVariantPairs(regression.variants)
  )
  if (
    regressionVariants.length === 0 ||
    regressionVariants.some(([k, v]) => requestedVariants[k] !== v)
  ) {
    mismatches.push('variants')
  }
  if (
    request.view &&
    !(regression.views || []).some((v) => v.view_name === request.view)
  ) {
    mismatches.push('view')
  }

  const closure = request.view
    ? historicalClosure(regression, request.view)
    : null
  let scope = closure?.scope
  if (!scope) {
    if (regression.closed?.Valid) scope = ClosureScope.REGRESSION
    else if (request.view) scope = ClosureScope.OPEN
    else scope = ClosureScope.UNKNOWN
  }
  return {
    valid: mismatches.length === 0,
    mismatches,
    scope,
    association: closure?.association ?? null,
  }
}
