import './ComponentReadiness.css'
import {
  cancelledDataTable,
  getAPIUrl,
  getColumns,
  gotFetchError,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { CompReadyVarsContext } from '../CompReadyVars'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { TableContainer, Tooltip, Typography } from '@material-ui/core'
import CompCapRow from './CompCapRow'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyPageTitle from './CompReadyPageTitle'
import CompReadyProgress from './CompReadyProgress'
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
  console.log('Aborting page2a')
  abortController.abort()
}

// This component runs when we see /component_readiness/env_capabilities
// This component runs when we see /component_readiness/capabilities
// This is page 2 or 2a which runs when you click a component cell on the left or under an environment of page 1.
export default function CompReadyEnvCapabilities(props) {
  const { filterVals, component, environment } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  // Set the browser tab title
  document.title =
    'Sippy > ComponentReadiness > Capabilities' + (environment ? `Env` : '')

  const { expandEnvironment } = useContext(CompReadyVarsContext)
  const safeComponent = safeEncodeURIComponent(component)

  const apiCallStr =
    getAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${safeComponent}` +
    (environment ? expandEnvironment(environment) : '')

  const newFilterVals =
    filterVals + `&component=${safeComponent}` + expandEnvironment(environment)

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
  }, [])

  if (fetchError !== '') {
    return gotFetchError(fetchError)
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
      <CompReadyPageTitle pageTitle={pageTitle} apiCallStr={apiCallStr} />
      <h2>
        <Link to="/component_readiness">/</Link>
        {environment ? `${environment} > ${component}` : component}
      </h2>
      <br></br>
      <TableContainer component="div" className="cr-table-wrapper">
        <Table className="cr-comp-read-table">
          <TableHead>
            <TableRow>
              <TableCell className={'cr-col-result-full'}>
                <Typography className="cr-cell-capab-col">Name</Typography>
              </TableCell>
              {columnNames.map((column, idx) => {
                if (column !== 'Name') {
                  return (
                    <TableCell
                      className={'cr-col-result'}
                      key={'column' + '-' + idx}
                    >
                      <Tooltip title={'Single row report for ' + column}>
                        <Typography className="cr-cell-name">
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
                  <CompCapRow
                    key={componentIndex}
                    capabilityName={data.rows[componentIndex].capability}
                    results={data.rows[componentIndex].columns}
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
    </Fragment>
  )
}

CompReadyEnvCapabilities.propTypes = {
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  environment: PropTypes.string,
}
