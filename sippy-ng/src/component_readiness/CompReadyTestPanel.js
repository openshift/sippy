import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { Grid, TableContainer, Tooltip, Typography } from '@mui/material'
import CompReadyTestDetailRow from './CompReadyTestDetailRow'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'

export default function CompReadyTestPanel(props) {
  const { data, isOverride, versions, loadedParams } = props
  const classes = useContext(ComponentReadinessStyleContext)

  const significanceTitle = `Test results for individual Prow Jobs may not be statistically
  significant, but when taken in aggregate, there may be a statistically
  significant difference compared to the historical basis
  `

  const [showOnlyFailures, setShowOnlyFailures] = React.useState(false)

  const tableCell = (label, idx) => {
    return (
      <TableCell className={classes.crColResult} key={'column' + '-' + idx}>
        <Typography className={classes.crCellName}>{label}</Typography>
      </TableCell>
    )
  }

  const tableTooltipCell = (label, idx, title) => {
    return (
      <Tooltip title={title}>
        <TableCell className={classes.crColResult} key={'column' + '-' + idx}>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <InfoIcon style={{ fontSize: '16px', fontWeight: 'lighter' }} />
            <Typography className={classes.crCellName}>{label}</Typography>
          </div>
        </TableCell>
      </Tooltip>
    )
  }

  const handleFailuresOnlyChange = (event) => {
    setShowOnlyFailures(event.target.checked)
  }

  const printParamsAndStats = (
    statsLabel,
    stats,
    from,
    to,
    vCrossCompare,
    variantSelection
  ) => {
    const summaryDate = getSummaryDate(from, to, stats.release, versions)
    return (
      <Fragment>
        {statsLabel} Release: <strong>{stats.release}</strong>
        {summaryDate && (
          <Fragment>
            <br />
            &nbsp;&nbsp;<strong>{summaryDate}</strong>
          </Fragment>
        )}
        <br />
        &nbsp;&nbsp;Start Time: <strong>{from}</strong>
        <br />
        &nbsp;&nbsp;End Time: <strong>{to}</strong>
        <br />
        {vCrossCompare && vCrossCompare.length ? (
          <Fragment>
            <br />
            &nbsp;&nbsp;Variant Cross Comparison:
            <ul>
              {vCrossCompare.map((group, idx) =>
                variantSelection[group] ? (
                  <li key={idx}>
                    {group}:&nbsp;
                    <strong>{variantSelection[group].join(', ')}</strong>
                  </li>
                ) : (
                  <li key={idx}>
                    {group}: <strong>(any)</strong>
                  </li>
                )
              )}
            </ul>
          </Fragment>
        ) : (
          ''
        )}
        <br />
        Statistics:
        <ul>
          <li>Success Rate: {(stats.success_rate * 100).toFixed(2)}%</li>
          <li>Successes: {stats.success_count}</li>
          <li>Failures: {stats.failure_count}</li>
          <li>Flakes: {stats.flake_count}</li>
        </ul>
      </Fragment>
    )
  }

  // getSummaryDate attempts to translate a date into text relative to the version GA
  // dates we know about.  If there are no versions, there is no translation.
  const getSummaryDate = (from, to, version, versions) => {
    const fromDate = new Date(from)
    const toDate = new Date(to)

    // Go through the versions map from latest release to earliest; ensure that
    // the ordering is by version (e.g., 4.6 is considered earlier than 4.10).
    const sortedVersions = Object.keys(versions).sort((a, b) => {
      const itemA = parseInt(a.toString().replace(/\./g, ''))
      const itemB = parseInt(b.toString().replace(/\./g, ''))
      return itemB - itemA
    })

    if (!versions[version]) {
      // Handle the case where GA date is undefined (implies under development and not GA)
      const weeksBefore = Math.floor(
        (toDate - fromDate) / (1000 * 60 * 60 * 24 * 7)
      )

      // Calculate the difference between now and toDate in hours
      const now = new Date()
      const hoursDifference = Math.abs(now - toDate) / (1000 * 60 * 60)

      if (hoursDifference <= 72) {
        return weeksBefore
          ? `Recent ${weeksBefore} week(s) of ${version}`
          : null
      } else {
        // Convert toDate to human-readable format
        const toDateFormatted = new Date(toDate).toLocaleDateString()
        return weeksBefore
          ? `${weeksBefore} week(s) before ${toDateFormatted} of ${version}`
          : null
      }
    }

    for (const version of sortedVersions) {
      if (!versions[version]) {
        // We already dealt with a version with no GA date above.
        continue
      }
      const gaDateStr = versions[version]
      const gaDate = new Date(gaDateStr)

      // Widen the window by 20 weeks prior to GA (because releases seems to be that long) and give
      // a buffer of 1 week after GA.
      const twentyWeeksPreGA = new Date(gaDate.getTime())
      twentyWeeksPreGA.setDate(twentyWeeksPreGA.getDate() - 20 * 7)
      gaDate.setDate(gaDate.getDate() + 7)

      if (fromDate >= twentyWeeksPreGA && toDate <= gaDate) {
        // Calculate the time (in milliseconds) to weeks
        const weeksBefore = Math.floor(
          (gaDate - fromDate) / (1000 * 60 * 60 * 24 * 7)
        )
        return weeksBefore
          ? `About ${weeksBefore} week(s) before '${version}' GA date`
          : null
      }
    }
    return null
  }

  let urls = []
  if (data?.incidents?.length) {
    data.incidents.forEach((incident) => {
      incident?.job_runs?.forEach((job_run) => {
        if (job_run.url !== undefined) {
          urls[job_run.url] = true
        }
      })
    })
  }

  return (
    <Fragment>
      <Grid container spacing={2} style={{ marginTop: '10px' }}>
        {data.base_stats && (
          <Grid item xs={6}>
            {printParamsAndStats(
              'Basis (historical)',
              data.base_stats,
              data.base_stats.Start.toString(),
              data.base_stats.End.toString(),
              loadedParams.variantCrossCompare,
              loadedParams.includeVariantsCheckedItems
            )}
          </Grid>
        )}
        {data.sample_stats && data.sample_stats.Start && data.sample_stats.End && (
          <Grid item xs={6}>
            {printParamsAndStats(
              'Sample (being evaluated)',
              data.sample_stats,
              data.sample_stats.Start.toString(),
              data.sample_stats.End.toString(),
              loadedParams.variantCrossCompare,
              loadedParams.compareVariantsCheckedItems
            )}
          </Grid>
        )}
      </Grid>
      <div style={{ marginTop: '10px', marginBottom: '10px' }}>
        <label>
          <input
            type="checkbox"
            checked={showOnlyFailures}
            onChange={handleFailuresOnlyChange}
          />
          Only Show Failures
        </label>
      </div>
      <TableContainer component="div" className="cr-table-wrapper">
        <Table className="cr-comp-read-table">
          <TableHead>
            <TableRow>
              {tableCell('Basis Job', 0)}
              {tableCell('Basis Runs', 1)}
              {tableCell('', 2)}
              {tableCell('Sample Job', 3)}
              {tableCell('Sample Runs', 4)}
              {tableTooltipCell(
                'Statistically Significant',
                5,
                significanceTitle
              )}
            </TableRow>
          </TableHead>
          <TableBody>
            {/* Ensure we have data before trying to map on it; we need data and rows */}
            {data && data.job_stats && data.job_stats.length > 0 ? (
              data.job_stats
                .sort((a, b) => {
                  if (a.significant && b.significant) {
                    return 0
                  } else if (a.significant) {
                    // This makes it so that statistically significant ones go to the top.
                    return -1
                  } else {
                    return 1
                  }
                })
                .map((element, idx) => {
                  return (
                    <CompReadyTestDetailRow
                      key={idx}
                      element={element}
                      idx={idx}
                      showOnlyFailures={showOnlyFailures}
                      triagedURLs={urls}
                    ></CompReadyTestDetailRow>
                  )
                })
            ) : (
              <TableRow>
                {/* No data to render (possible due to a Cancel */}
                <TableCell align="center">No data ; reload to retry</TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Fragment>
  )
}

CompReadyTestPanel.propTypes = {
  data: PropTypes.object.isRequired,
  versions: PropTypes.object.isRequired,
  isOverride: PropTypes.bool.isRequired,
  loadedParams: PropTypes.object.isRequired,
}
