import './ComponentReadiness.css'
import {
  cancelledDataTable,
  expandEnvironment,
  getAPIUrl,
  getColumns,
  gotFetchError,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { TableContainer, Tooltip, Typography } from '@material-ui/core'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyProgress from './CompReadyProgress'
import CompTestRow from './CompTestRow'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import Table from '@material-ui/core/Table'
import TableBody from '@material-ui/core/TableBody'
import TableCell from '@material-ui/core/TableCell'
import TableHead from '@material-ui/core/TableHead'
import TableRow from '@material-ui/core/TableRow'

// Big query requests take a while so give the user the option to
// abort in case they inadvertently requested a huge dataset.
let abortController = new AbortController()
const cancelFetch = () => {
  console.log('Aborting page3')
  abortController.abort()
}

// This component runs when we see /component_readiness/env_capability
// This is page 3a which runs when you click a status cell under an environment on the right in page 2 or 2a
export default function CompReadyEnvCapability(props) {
  const { filterVals, component, capability, environment } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  // Set the browser tab title
  document.title = `EnvCapability`

  const safeComponent = safeEncodeURIComponent(component)
  const safeCapability = safeEncodeURIComponent(capability)

  const apiCallStr =
    getAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${safeComponent}` +
    `&capability=${safeCapability}` +
    expandEnvironment(environment)

  useEffect(() => {
    setIsLoaded(false)
    const fromFile = false
    if (fromFile) {
      console.log('FILE')
    } else {
      console.log('about to fetch page3a: ', apiCallStr)
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
            console.log('Request was cancelled')
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
  }, [])

  if (fetchError !== '') {
    return gotFetchError(fetchError)
  }

  const pageTitle = (
    <Typography variant="h4" style={{ margin: 20, textAlign: 'center' }}>
      Test report for environment ({environment}) component ({component})
      capability ({capability}) page 3a
    </Typography>
  )

  if (!isLoaded) {
    return <CompReadyProgress apiLink={apiCallStr} cancelFunc={cancelFetch} />
  }

  const columnNames = getColumns(data)
  if (columnNames[0] === 'Cancelled' || columnNames[0] == 'None') {
    return <CompReadyCancelled />
  }

  return (
    <Fragment>
      {pageTitle}
      <h2>
        <Link to="/component_readiness">/</Link> {component} &gt; {capability}
      </h2>
      <br></br>
      <TableContainer component="div" className="cr-wrapper">
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
                  <CompTestRow
                    key={componentIndex}
                    testName={data.rows[componentIndex].test_name}
                    testId={data.rows[componentIndex].test_id}
                    results={data.rows[componentIndex].columns}
                    columnNames={columnNames}
                    filterVals={filterVals}
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

CompReadyEnvCapability.propTypes = {
  filterVals: PropTypes.string,
  component: PropTypes.string,
  capability: PropTypes.string,
  environment: PropTypes.string,
}
