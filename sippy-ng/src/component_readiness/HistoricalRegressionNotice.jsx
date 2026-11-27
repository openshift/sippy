import { Alert, AlertTitle, Box } from '@mui/material'
import {
  ClosureScope,
  formatUrlDateRange,
  LookupStatus,
} from './HistoricalRegressionUtils'
import { formatDateToSeconds } from '../helpers'
import PropTypes from 'prop-types'
import React from 'react'

const titles = {
  [ClosureScope.REGRESSION]: 'Closed regression, historical report',
  [ClosureScope.VIEW]: 'Closed in this view, historical report',
  [ClosureScope.OPEN]: 'Historical report for an open regression',
  [ClosureScope.UNKNOWN]: 'Historical report for a tracked regression',
}

// HistoricalWarning reports why closure details cannot be shown for the
// regression a historical link names.
function HistoricalWarning({ children }) {
  return (
    <Alert severity="warning" sx={{ my: 2 }} data-testid="historical-notice">
      {children}
    </Alert>
  )
}

HistoricalWarning.propTypes = {
  children: PropTypes.node,
}

// HistoricalRegressionNotice explains the closure context of the regression a
// historical test details link reconstructs. Closure details come from the
// regression API record and are shown only after it has been matched to this
// report's test, release, variants, and view. The statistical assessment below
// the notice is left untouched, even when it is red.
export default function HistoricalRegressionNotice({
  regressionId,
  lookup,
  validation,
  urlParams,
  noData,
}) {
  // idle covers the render before the lookup effect starts, such as when
  // navigation adds regressionId to an already mounted report. A loaded record
  // for a different ID is likewise stale until the lookup effect replaces it.
  if (
    !regressionId ||
    lookup.status === LookupStatus.IDLE ||
    lookup.status === LookupStatus.LOADING ||
    (lookup.status === LookupStatus.LOADED &&
      String(lookup.regression?.id) !== String(regressionId))
  ) {
    return null
  }

  if (lookup.status === LookupStatus.MISSING) {
    return (
      <HistoricalWarning>
        Regression {regressionId} was not found, so this report cannot be
        identified as the historical report of a closed regression.
      </HistoricalWarning>
    )
  }
  if (lookup.status === LookupStatus.ERROR) {
    return (
      <HistoricalWarning>
        Unable to load regression {regressionId}: {lookup.error}
      </HistoricalWarning>
    )
  }
  if (!validation) return null
  if (!validation.valid) {
    return (
      <HistoricalWarning>
        Regression {regressionId} does not match this report (
        {validation.mismatches.join(', ')}), so its closure details are not
        shown.
      </HistoricalWarning>
    )
  }

  const regression = lookup.regression
  const { association } = validation
  const sampleRange = formatUrlDateRange(
    urlParams.sampleStartTime,
    urlParams.sampleEndTime
  )
  const basisRange = formatUrlDateRange(
    urlParams.baseStartTime,
    urlParams.baseEndTime
  )

  let closedText = 'Open'
  if (validation.scope === ClosureScope.REGRESSION) {
    closedText = formatDateToSeconds(regression.closed.Time)
  } else if (validation.scope === ClosureScope.UNKNOWN) {
    closedText =
      'Not closed overall; closure in a specific view is unknown because this report has no view'
  } else if (validation.scope === ClosureScope.VIEW) {
    closedText = association.closed_at?.Valid
      ? `Closed in ${urlParams.view} on ${formatDateToSeconds(
          association.closed_at.Time
        )}; still open in another view`
      : `Closed in ${urlParams.view}; still open in another view`
  }

  return (
    <Alert severity="info" sx={{ my: 2 }} data-testid="historical-notice">
      <AlertTitle>{titles[validation.scope]}</AlertTitle>
      <Box component="ul" sx={{ m: 0, pl: 2 }}>
        <li>Regression ID: {regression.id}</li>
        <li>Closed: {closedText}</li>
        <li>
          Last failure:{' '}
          {regression.last_failure?.Valid
            ? formatDateToSeconds(regression.last_failure.Time)
            : 'not recorded'}
        </li>
        <li>
          Sample window: {sampleRange ? `${sampleRange} (UTC)` : 'unknown'}
        </li>
        <li>Basis window: {basisRange ? `${basisRange} (UTC)` : 'unknown'}</li>
      </Box>
      {noData ? (
        <Box sx={{ mt: 1 }}>
          No test results are available for this historical window. The closure
          details above come from the persisted regression record.
        </Box>
      ) : (
        <Box sx={{ mt: 1 }}>
          The assessment below is the statistical result for this historical
          window and may still show a regression. Closure reflects regression
          tracking, not a passing result.
        </Box>
      )}
    </Alert>
  )
}

HistoricalRegressionNotice.propTypes = {
  regressionId: PropTypes.number,
  lookup: PropTypes.shape({
    status: PropTypes.oneOf(Object.values(LookupStatus)).isRequired,
    regression: PropTypes.object,
    error: PropTypes.string,
  }).isRequired,
  validation: PropTypes.shape({
    valid: PropTypes.bool,
    mismatches: PropTypes.array,
    scope: PropTypes.oneOf(Object.values(ClosureScope)),
    association: PropTypes.object,
  }),
  urlParams: PropTypes.object.isRequired,
  noData: PropTypes.bool,
}
