import { Alert, AlertTitle, Box } from '@mui/material'
import { formatDateToSeconds } from '../helpers'
import { formatUrlDateRange } from './CompReadyUtils'
import PropTypes from 'prop-types'
import React from 'react'

const titles = {
  regression: 'Closed regression, historical report',
  view: 'Closed in this view, historical report',
  open: 'Historical report for an open regression',
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
  view,
  noData,
}) {
  if (!regressionId || lookup.status === 'loading') return null

  if (lookup.status === 'missing') {
    return (
      <Alert severity="warning" sx={{ my: 2 }} data-testid="historical-notice">
        Regression {regressionId} was not found, so this report cannot be
        identified as the historical report of a closed regression.
      </Alert>
    )
  }
  if (lookup.status === 'error') {
    return (
      <Alert severity="warning" sx={{ my: 2 }} data-testid="historical-notice">
        Unable to load regression {regressionId}: {lookup.error}
      </Alert>
    )
  }
  if (!validation.valid) {
    return (
      <Alert severity="warning" sx={{ my: 2 }} data-testid="historical-notice">
        Regression {regressionId} does not match this report (
        {validation.mismatches.join(', ')}), so its closure details are not
        shown.
      </Alert>
    )
  }

  const regression = lookup.regression
  const association = (regression.views || []).find((v) => v.view_name === view)
  const sampleRange = formatUrlDateRange(
    urlParams.sampleStartTime,
    urlParams.sampleEndTime
  )
  const basisRange = formatUrlDateRange(
    urlParams.baseStartTime,
    urlParams.baseEndTime
  )

  let closedText = 'Open'
  if (validation.scope === 'regression') {
    closedText = formatDateToSeconds(regression.closed.Time)
  } else if (validation.scope === 'view') {
    closedText = association?.closed_at?.Valid
      ? `Closed in ${view} on ${formatDateToSeconds(
          association.closed_at.Time
        )}; still open in another view`
      : `Closed in ${view}; still open in another view`
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
    status: PropTypes.oneOf(['idle', 'loading', 'loaded', 'missing', 'error'])
      .isRequired,
    regression: PropTypes.object,
    error: PropTypes.string,
  }).isRequired,
  validation: PropTypes.shape({
    valid: PropTypes.bool,
    mismatches: PropTypes.array,
    scope: PropTypes.string,
  }),
  urlParams: PropTypes.object.isRequired,
  view: PropTypes.string,
  noData: PropTypes.bool,
}
