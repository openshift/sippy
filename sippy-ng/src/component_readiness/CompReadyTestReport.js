import './ComponentReadiness.css'
import {
  cancelledDataTable,
  getColumns,
  getStatusAndIcon,
  getTestDetailsAPIUrl,
  gotFetchError,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { CompReadyVarsContext } from './CompReadyVars'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { TableContainer, Typography } from '@material-ui/core'
import { Tooltip } from '@material-ui/core'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyPageTitle from './CompReadyPageTitle'
import CompReadyProgress from './CompReadyProgress'
import CompReadyTestDetailRow from './CompReadyTestDetailRow'
import InfoIcon from '@material-ui/icons/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useEffect } from 'react'
import Table from '@material-ui/core/Table'
import TableBody from '@material-ui/core/TableBody'
import TableCell from '@material-ui/core/TableCell'
import TableHead from '@material-ui/core/TableHead'
import TableRow from '@material-ui/core/TableRow'

// Big query requests take a while so give the user the option to
// abort in case they inadvertently requested a huge dataset.
let abortController = new AbortController()
const cancelFetch = () => {
  console.log('Aborting page5a')
  abortController.abort()
}

const printStats = (statsLabel, stats) => {
  return (
    <Fragment>
      {statsLabel} Release: {stats.release}
      <ul>
        <li>Success Rate: {(stats.success_rate * 100).toFixed(2)}%</li>
        <li>Successes: {stats.success_count}</li>
        <li>Failures: {stats.failure_count}</li>
        <li>Flakes: {stats.flake_count}</li>
      </ul>
    </Fragment>
  )
}

const tableCell = (label, idx) => {
  return (
    <TableCell className={'cr-col-result'} key={'column' + '-' + idx}>
      <Typography className="cr-cell-name">{label}</Typography>
    </TableCell>
  )
}
const tableTooltipCell = (label, idx, title) => {
  return (
    <Tooltip title={title}>
      <TableCell className={'cr-col-result'} key={'column' + '-' + idx}>
        <div style={{ display: 'flex', alignItems: 'center' }}>
          <InfoIcon style={{ fontSize: '16px', fontWeight: 'lighter' }} />
          <Typography className="cr-cell-name">{label}</Typography>
        </div>
      </TableCell>
    </Tooltip>
  )
}

// This component runs when we see /component_readiness/test_details
// This is page 5 which runs when you click a test cell on the right of page 4 or page 4a
export default function CompReadyTestReport(props) {
  const { filterVals, component, capability, testId, environment, testName } =
    props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [showOnlyFailures, setShowOnlyFailures] = React.useState(false)

  // Set the browser tab title
  document.title =
    'Sippy > ComponentReadiness > Capabilities > Tests > Capability Tests > TestDetails' +
    (environment ? `Env` : '')
  const safeComponent = safeEncodeURIComponent(component)
  const safeCapability = safeEncodeURIComponent(capability)
  const safeTestId = safeEncodeURIComponent(testId)

  const { expandEnvironment } = useContext(CompReadyVarsContext)

  const apiCallStr =
    getTestDetailsAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${safeComponent}` +
    `&capability=${safeCapability}` +
    `&testId=${safeTestId}` +
    (environment ? expandEnvironment(environment) : '')

  useEffect(() => {
    setIsLoaded(false)
    fetch(apiCallStr, { signal: abortController.signal })
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        // If the basics are not present, consider it no data
        if (!json.component || !json.sample_stats || !json.base_stats) {
          // The api call returned 200 OK but the data was empty
          setData(noDataTable)
        } else {
          setData(json)
        }
      })
      .catch((error) => {
        if (error.name === 'AbortError') {
          setData(cancelledDataTable)

          // Once this fired, we need a new one for the next button click.
          abortController = new AbortController()
        } else {
          setFetchError(`API call failed: ${apiCallStr}\n${error}`)
        }
      })
      .finally(() => {
        // Mark the attempt as finished whether successful or not.
        setIsLoaded(true)
      })
  }, [])

  if (fetchError !== '') {
    return gotFetchError(fetchError)
  }

  const pageTitle = makePageTitle(
    'Test Details Report',
    environment ? 'page 5a' : 'page 5',
    `component: ${component}`,
    `capability: ${capability}`,
    `testId: ${testId}`,
    `testName: ${testName}`,
    `environment: ${environment}`
  )

  if (!isLoaded) {
    return <CompReadyProgress apiLink={apiCallStr} cancelFunc={cancelFetch} />
  }

  const columnNames = getColumns(data)
  if (columnNames[0] === 'Cancelled' || columnNames[0] == 'None') {
    return (
      <CompReadyCancelled message={columnNames[0]} apiCallStr={apiCallStr} />
    )
  }

  // Customize the maxJobNumFactor based on number of potential number of JobRuns.
  // The display will get wide so max out at 100 jobs.
  let maxLength = 0
  data.job_stats.forEach((item) => {
    if (item.base_job_run_stats && item.base_job_run_stats.length > maxLength) {
      maxLength = item.base_job_run_stats.length
    }
    if (
      item.sample_job_run_stats &&
      item.sample_job_run_stats.length > maxLength
    ) {
      maxLength = item.sample_job_run_stats.length
    }
  })
  const maxJobNumFactor = Math.min(maxLength / 10 + 1, 10)
  const marks = Array.from({ length: maxJobNumFactor }, (_, index) => ({
    value: index + 1,
    label: (index + 1).toString(),
  }))

  const handleFailuresOnlyChange = (event) => {
    setShowOnlyFailures(event.target.checked)
  }

  const probabilityStr = (statusStr, fisherNumber) => {
    if (
      statusStr.includes('SignificantRegression') ||
      statusStr.includes('ExtremeRegression')
    ) {
      return `Probability of significant regression: ${(
        (1 - fisherNumber) *
        100
      ).toFixed(2)}%`
    } else if (statusStr.includes('SignificantImprovement')) {
      return `Probability of significant improvement: ${(
        (1 - fisherNumber) *
        100
      ).toFixed(2)}%`
    } else {
      return 'There is no significant evidence of regression'
    }
  }

  const [statusStr, assessmentIcon] = getStatusAndIcon(data.report_status)
  const significanceTitle = `Test results for individual Prow Jobs may not be statistically
  significant, but when taken in aggregate, there may be a statistically
  significant difference compared to the historical basis
  `

  return (
    <Fragment>
      <CompReadyPageTitle pageTitle={pageTitle} apiCallStr={apiCallStr} />
      <h3>
        <Link to="/component_readiness">
          / {environment} &gt; {component}
          &gt; {testName}{' '}
        </Link>
      </h3>
      Test Name: {testName}
      <hr />
      {printStats('Sample (being evaluated)', data.sample_stats)}
      {printStats('Base (historical)', data.base_stats)}
      <Fragment>
        <div style={{ display: 'block' }}>Environment: {environment}</div>
        <br />
        Assessment: <Tooltip title={statusStr}>{assessmentIcon}</Tooltip>
        <br />
        <div style={{ display: 'block' }}>
          {/* data.fisher_exact is from 0-1; from that we calculate the probability of regression
              expressed as a percentage */}
          <Tooltip
            title={`Fisher Exact Number for this basis and sample = ${data.fisher_exact}`}
          >
            <InfoIcon />
          </Tooltip>
          {probabilityStr(statusStr, data.fisher_exact)}
        </div>
        <br />
      </Fragment>
      <hr />
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
              {tableCell('ProwJob Name', 0)}
              {tableCell('Basis Info', 1)}
              {tableCell('Basis Runs', 2)}
              {tableCell('Sample Info', 3)}
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
              data.job_stats.map((element, idx) => {
                return (
                  <CompReadyTestDetailRow
                    key={idx}
                    element={element}
                    idx={idx}
                    showOnlyFailures={showOnlyFailures}
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

CompReadyTestReport.propTypes = {
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  environment: PropTypes.string.isRequired,
  testName: PropTypes.string.isRequired,
}
