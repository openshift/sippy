import './ComponentReadiness.css'
import { BooleanParam, StringParam, useQueryParam } from 'use-query-params'
import {
  cancelledDataTable,
  formColumnName,
  getColumns,
  getCRMainAPIUrl,
  gotFetchError,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { escapeRegex, safeEncodeURIComponent } from '../helpers'
import { Link } from 'react-router-dom'
import { TableContainer, Tooltip, Typography } from '@mui/material'
import CompCapRow from './CompCapRow'
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
  console.log('Aborting page2a')
  abortController.abort()
}

// This component runs when we see /component_readiness/env_capabilities
// This component runs when we see /component_readiness/capabilities
// This is page 2 or 2a which runs when you click a component cell on the left or under an environment of page 1.
export default function CompReadyEnvCapabilities(props) {
  const classes = useContext(ComponentReadinessStyleContext)
  const { filterVals, component, environment, theme } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  const [triageActionTaken, setTriageActionTaken] = React.useState(false)

  // Set the browser tab title
  document.title =
    'Sippy > Component Readiness > Capabilities' +
    (environment ? ` by Environment` : '')

  const { expandEnvironment } = useContext(CompReadyVarsContext)
  const safeComponent = safeEncodeURIComponent(component)

  let apiCallStr =
    getCRMainAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${safeComponent}` +
    (environment ? expandEnvironment(environment) : '')

  const newFilterVals =
    filterVals + `&component=${safeComponent}` + expandEnvironment(environment)

  useEffect(() => {
    setIsLoaded(false)
    fetchData()
    setTriageActionTaken(false)
  }, [triageActionTaken])

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
          console.log('got empty page2', json)
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

  const forceRefresh = () => {
    setIsLoaded(false)
    fetchData(true)
  }
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

  const pageTitle = makePageTitle(
    'Capabilities report' + (environment ? ', Environment' : ''),
    environment ? 'page 2a' : 'page2',
    `environment: ${environment}`,
    `component: ${component}`,
    `rows: ${data && data.rows ? data.rows.length : 0}, columns: ${
      data && data.rows && data.rows[0] && data.rows[0].columns
        ? data.rows[0].columns.length
        : 0
    }`
  )

  const columnNames = getColumns(data)

  if (fetchError !== '') {
    return gotFetchError(fetchError)
  }

  if (!isLoaded) {
    return <CompReadyProgress apiLink={apiCallStr} cancelFunc={cancelFetch} />
  }

  if (columnNames[0] === 'Cancelled' || columnNames[0] == 'None') {
    return (
      <CompReadyCancelled message={columnNames[0]} apiCallStr={apiCallStr} />
    )
  }

  return (
    <Fragment>
      <Sidebar theme={theme} />
      <CompReadyPageTitle pageTitle={pageTitle} apiCallStr={apiCallStr} />
      <h2>
        <Link to="/component_readiness">/</Link>
        {environment ? `${environment} > ${component}` : component}
      </h2>
      <ComponentReadinessToolBar
        searchRowRegex={searchRowRegex}
        handleSearchRowRegexChange={handleSearchRowRegexChange}
        searchColumnRegex={searchColumnRegex}
        handleSearchColumnRegexChange={handleSearchColumnRegexChange}
        redOnlyChecked={redOnlyChecked}
        handleRedOnlyCheckboxChange={handleRedOnlyCheckboxChange}
        clearSearches={clearSearches}
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
              {columnNames
                .filter((column, idx) =>
                  column.match(new RegExp(escapeRegex(searchColumnRegex), 'i'))
                )
                .map((column, idx) => {
                  if (column !== 'Name') {
                    return (
                      <TableCell
                        className={classes.crColResult}
                        key={'column' + '-' + idx}
                      >
                        <Tooltip title={'Single row report for ' + column}>
                          <Typography className={classes.crColName}>
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
              Object.keys(data.rows)
                .filter((rowIndex) =>
                  data.rows[rowIndex].capability.match(
                    new RegExp(escapeRegex(searchRowRegex), 'i')
                  )
                )
                .filter((rowIndex) =>
                  redOnlyChecked
                    ? data.rows[rowIndex].columns.some(
                        // Filter for rows where any of their columns have status <= -2 and accepted by the regex.
                        (column) =>
                          column.status <= -2 &&
                          formColumnName(column).match(
                            new RegExp(escapeRegex(searchColumnRegex), 'i')
                          )
                      )
                    : true
                )
                .map((componentIndex) => {
                  return (
                    <CompCapRow
                      key={componentIndex}
                      capabilityName={data.rows[componentIndex].capability}
                      results={data.rows[componentIndex].columns.filter(
                        (column, idx) =>
                          formColumnName(column).match(
                            new RegExp(escapeRegex(searchColumnRegex), 'i')
                          )
                      )}
                      columnNames={columnNames}
                      filterVals={newFilterVals}
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

CompReadyEnvCapabilities.propTypes = {
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  environment: PropTypes.string,
  theme: PropTypes.object.isRequired,
}
