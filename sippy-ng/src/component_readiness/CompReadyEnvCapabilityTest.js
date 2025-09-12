import './ComponentReadiness.css'
import { BooleanParam, StringParam, useQueryParam } from 'use-query-params'
import {
  cancelledDataTable,
  getColumns,
  getCRMainAPIUrl,
  gotFetchError,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { TableContainer, Tooltip, Typography } from '@mui/material'
import CompCapTestRow from './CompCapTestRow'
import ComponentReadinessToolBar from './ComponentReadinessToolBar'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyPageTitle from './CompReadyPageTitle'
import CompReadyProgress from './CompReadyProgress'
import CopyPageURL from './CopyPageURL'
import GeneratedAt from './GeneratedAt'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useEffect, useState } from 'react'
import Sidebar from './Sidebar'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'

// Big query requests take a while so give the user the option to
// abort in case they inadvertently requested a huge dataset.
let abortController = new AbortController()
const cancelFetch = () => {
  console.log('Aborting page4')
  abortController.abort()
}

// This component runs when we see /component_readiness/env_test
// This component runs when we see /component_readiness/test
// This is page 4 or page 4a which runs when you click a capability cell on the left or cell on the right in page 3 or 3a
export default function CompReadyEnvCapabilityTest(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  const { filterVals, component, capability, testId, environment } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  const [triageActionTaken, setTriageActionTaken] = React.useState(false)

  const [searchRowRegexURL, setSearchRowRegexURL] = useQueryParam(
    'searchRow',
    StringParam
  )
  const [searchRowRegex, setSearchRowRegex] = useState(searchRowRegexURL)
  const handleSearchRowRegexChange = (event) => {
    const searchValue = event.target.value
    setSearchRowRegex(searchValue)
  }

  const [searchColumnRegexURL, setSearchColumnRegexURL] = useQueryParam(
    'searchColumn',
    StringParam
  )
  const [searchColumnRegex, setSearchColumnRegex] =
    useState(searchColumnRegexURL)
  const handleSearchColumnRegexChange = (event) => {
    const searchValue = event.target.value
    setSearchColumnRegex(searchValue)
  }

  const [redOnlyURL = false, setRedOnlyURL] = useQueryParam(
    'redOnly',
    BooleanParam
  )
  const [redOnlyChecked, setRedOnlyChecked] = React.useState(redOnlyURL)
  const handleRedOnlyCheckboxChange = (event) => {
    setRedOnlyChecked(event.target.checked)
  }

  const clearSearches = () => {
    setSearchRowRegex('')
    if (searchRowRegexURL && searchRowRegexURL !== '') {
      setSearchRowRegexURL('')
    }

    setSearchColumnRegex('')
    if (searchColumnRegexURL && searchColumnRegexURL !== '') {
      setSearchColumnRegexURL('')
    }

    if (setRedOnlyChecked) {
      setRedOnlyURL(false)
    }
    setRedOnlyChecked(false)
  }

  // Set the browser tab title
  document.title =
    'Sippy > Component Readiness > Capabilities > Tests > Capability Tests' +
    (environment ? ` by Environment` : '')
  const safeComponent = safeEncodeURIComponent(component)
  const safeCapability = safeEncodeURIComponent(capability)
  const safeTestId = safeEncodeURIComponent(testId)

  const { expandEnvironment, urlParams } = useContext(CompReadyVarsContext)
  let apiCallStr =
    getCRMainAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${safeComponent}` +
    `&capability=${safeCapability}` +
    `&testId=${safeTestId}` +
    (environment ? expandEnvironment(environment) : '')

  const forceRefresh = () => {
    setIsLoaded(false)
    fetchData(true)
  }

  useEffect(() => {
    setIsLoaded(false)
    if (!testId) return // wait until vars are initialized from params
    fetchData()
    setTriageActionTaken(false)
  }, [triageActionTaken, urlParams, testId])

  const fetchData = (fresh) => {
    if (fresh) {
      apiCallStr += '&forceRefresh=true'
    }

    fetch(apiCallStr, { signal: abortController.signal })
      .then((response) => response.json())
      .then((data) => {
        if (data.code < 200 || data.code >= 300) {
          const errorMessage = data.message
            ? `${data.message}`
            : 'No error message'
          throw new Error(`Return code = ${data.code} (${errorMessage})`)
        }
        return data
      })
      .then((json) => {
        if (Object.keys(json).length === 0 || json.rows.length === 0) {
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
  }

  if (fetchError !== '') {
    return gotFetchError(fetchError)
  }

  const pageTitle = makePageTitle(
    'Capability Test Report' + (environment ? ', Environment' : ''),
    environment ? 'page 4a' : 'page 4',
    `component: ${component}`,
    `capability: ${capability}`,
    `testId: ${testId}`,
    `environment: ${environment}`,
    `rows: ${data && data.rows ? data.rows.length : 0}, columns: ${
      data && data.rows && data.rows[0] && data.rows[0].columns
        ? data.rows[0].columns.length
        : 0
    }`
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

  return (
    <Fragment>
      <Sidebar />
      <CompReadyPageTitle pageTitle={pageTitle} apiCallStr={apiCallStr} />
      <h2>
        <Link to="/component_readiness">/</Link> {component} &gt; {capability}
      </h2>
      <ComponentReadinessToolBar
        data={data}
        filterVals={filterVals}
        setTriageActionTaken={setTriageActionTaken}
        forceRefresh={forceRefresh}
      />
      <br></br>
      <TableContainer component="div" className="cr-table-wrapper">
        <Table className="cr-comp-read-table">
          <TableHead>
            <TableRow>
              <TableCell className={classes.crColResultFull}>
                <Typography className={classes.crCellCapabCol}>Name</Typography>
              </TableCell>
              {columnNames.map((column, idx) => {
                if (column !== 'Name') {
                  return (
                    <TableCell
                      className={classes.crColResult}
                      key={'column' + '-' + idx}
                    >
                      <Tooltip title={'Single row report for ' + column}>
                        <Typography className={classes.crCellName}>
                          {column}
                        </Typography>
                      </Tooltip>
                    </TableCell>
                  )
                }
              })}
            </TableRow>
          </TableHead>
          <TableBody>
            {/* Ensure we have data before trying to map on it; we need data and rows */}
            {data && data.rows && Object.keys(data.rows).length > 0 ? (
              Object.keys(data.rows).map((componentIndex) => {
                return (
                  <CompCapTestRow
                    key={componentIndex}
                    testSuite={data.rows[componentIndex].test_suite}
                    testName={data.rows[componentIndex].test_name}
                    testId={data.rows[componentIndex].test_id}
                    results={data.rows[componentIndex].columns}
                    columnNames={columnNames}
                    filterVals={filterVals}
                    component={component}
                    capability={capability}
                  />
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
      <GeneratedAt time={data.generated_at} />
      <CopyPageURL apiCallStr={apiCallStr} />
    </Fragment>
  )
}

CompReadyEnvCapabilityTest.propTypes = {
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  environment: PropTypes.string,
}
