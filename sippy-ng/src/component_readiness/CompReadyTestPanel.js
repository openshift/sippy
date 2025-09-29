import {
  Box,
  Button,
  Dialog,
  FormControlLabel,
  FormGroup,
  Grid,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { getTestStatus } from '../helpers'
import CompReadyTestDetailRow from './CompReadyTestDetailRow'
import InfoIcon from '@mui/icons-material/Info'
import JobArtifactQuery from './JobArtifactQuery'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'

export default function CompReadyTestPanel(props) {
  const { data, versions, loadedParams, testName, environment, component } =
    props
  const classes = useContext(ComponentReadinessStyleContext)

  const significanceTitle = `Test results for individual Prow Jobs may not be statistically
  significant, but when taken in aggregate, there may be a statistically
  significant difference compared to the historical basis
  `

  const [showOnlyFailures, setShowOnlyFailures] = React.useState(false)
  const [searchJobArtifacts, setSearchJobArtifacts] = React.useState(false)

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

  const handleSearchJobArtifactsChange = (event) => {
    setSearchJobArtifacts(event.target.checked)
  }

  // combine data for job runs to enable the job artifact query
  const jobRuns = new Map()
  const initialSelectedRuns = new Set() // start with all sample failures
  for (const rowStat of data.job_stats || []) {
    let base = {
      job_name: rowStat.base_job_name,
      stats: rowStat.base_job_run_stats,
    }
    let sample = {
      job_name: rowStat.sample_job_name,
      stats: rowStat.sample_job_run_stats,
    }
    for (const jobStat of [base, sample]) {
      if (jobStat.stats && jobStat.stats.length > 0) {
        for (const jobRun of jobStat.stats) {
          jobRuns.set(jobRun.job_run_id, {
            job_run_id: jobRun.job_run_id,
            job_name: jobStat.job_name,
            start_time: jobRun.start_time,
            test_status: getTestStatus(
              jobRun.test_stats,
              'Flake',
              'Failure',
              'Success'
            ),
            url: jobRun.job_url,
          })
          if (jobStat === sample && jobRun.test_stats.failure_count > 0) {
            initialSelectedRuns.add(jobRun.job_run_id)
          }
        }
      }
    }
  }
  const [searchJobRunIds, setSearchJobRunIds] =
    React.useState(initialSelectedRuns)

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

  const [openJAQ, setOpenJAQ] = React.useState(false)
  function handleToggleJAQOpen(event) {
    setOpenJAQ(!openJAQ)
    event.target.blur() // otherwise button looks pressed on return
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
        <FormGroup row>
          <FormControlLabel
            label="Only show failures"
            control={
              <Switch
                color="primary"
                checked={showOnlyFailures}
                onChange={handleFailuresOnlyChange}
              />
            }
          />
          <FormControlLabel
            label="Select runs to compare"
            labelPlacement="end"
            control={
              <Switch
                color="primary"
                checked={searchJobArtifacts}
                onChange={handleSearchJobArtifactsChange}
              />
            }
          />
          <Button
            size="small"
            variant="contained"
            color="primary"
            onClick={handleToggleJAQOpen}
          >
            {searchJobArtifacts
              ? 'Compare Selected'
              : 'Compare Sample Failures'}
          </Button>
        </FormGroup>
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
                      searchJobArtifacts={searchJobArtifacts}
                      searchJobRunIds={searchJobRunIds}
                      setSearchJobRunIds={setSearchJobRunIds}
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
      {searchJobArtifacts && (
        <Button
          size="small"
          variant="contained"
          color="primary"
          onClick={handleToggleJAQOpen}
        >
          Compare selected
        </Button>
      )}
      <Dialog
        fullWidth={true}
        maxWidth={false}
        open={openJAQ}
        onClose={handleToggleJAQOpen}
      >
        <Grid className="jaq-dialog" tabIndex="0">
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              mb: 2,
            }}
          >
            <Typography variant="h4" component="h1">
              Test Details Job Artifact Query
            </Typography>
            <Tooltip title="Return to details report">
              <Button
                size="large"
                variant="contained"
                onClick={handleToggleJAQOpen}
              >
                Close
              </Button>
            </Tooltip>
          </Box>

          <Box sx={{ mb: 2 }}>
            <Typography variant="body1" sx={{ mb: 1 }}>
              <strong>Test name:</strong> {testName}
            </Typography>
            <Typography variant="body1" sx={{ mb: 1 }}>
              <strong>Variants:</strong> {environment}
            </Typography>
            <Typography variant="body1" sx={{ mb: 1 }}>
              <strong>Component:</strong> {component}
            </Typography>
          </Box>

          <Box sx={{ flex: 1 }}>
            <JobArtifactQuery
              searchJobRunIds={searchJobRunIds}
              jobRunsLookup={jobRuns}
              handleToggleJAQOpen={handleToggleJAQOpen}
            />
          </Box>
        </Grid>
      </Dialog>
    </Fragment>
  )
}

CompReadyTestPanel.propTypes = {
  data: PropTypes.object.isRequired,
  versions: PropTypes.object.isRequired,
  loadedParams: PropTypes.object.isRequired,
  testName: PropTypes.string.isRequired,
  environment: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
}
