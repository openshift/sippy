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
import { StringParam, useQueryParam } from 'use-query-params'
import { TableContainer, Tooltip, Typography } from '@material-ui/core'
import { useHistory } from 'react-router-dom'
import CompCapRow from './CompCapRow'
import CompReadyProgress from './CompReadyProgress'
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
  console.log('Aborting page2a')
  abortController.abort()
}

// This component runs when we see /component_readiness/cap_environment
// This is page 2a which runs when you click a component cell under an environment of page 1.
export default function CompReadyEnvCapabilities(props) {
  const filterVals = props.filterVals

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  // Set the browser tab title
  document.title = `CompReadEnv Test`

  const [componentParam] = useQueryParam('component', StringParam)
  const [environmentParam] = useQueryParam('environment', StringParam)
  const comp = componentParam || ''
  let env = environmentParam || ''

  if (filterVals.includes('environment')) {
    env = ''
  }
  const apiCallStr =
    getAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${comp}` +
    expandEnvironment(env)

  const newFilterVals =
    filterVals + `&component=${comp}` + expandEnvironment(env)

  useEffect(() => {
    setIsLoaded(false)
    const fromFile = false
    if (fromFile) {
      console.log('FILE')
      if (!(comp === '[sig-auth]' && env == 'ovn amd64 aws')) {
        console.log('no data for', comp, env)
        setData(noDataTable)
      } else {
        const json = require('./api_page2-sig-auth-ovn-amd-aws.json')
        setData(json)
        console.log('json (page2):', json)
      }
      setIsLoaded(true)
    } else {
      console.log('about to fetch page2: ', apiCallStr)
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

  let envDisplay = ''

  if (env != null) {
    envDisplay = ` ${env}`
  }
  const pageTitle = (
    <Typography variant="h4" style={{ margin: 20, textAlign: 'center' }}>
      Capabilities report for environment ({envDisplay}) component ({comp}) page
      2a
    </Typography>
  )

  if (!isLoaded) {
    return <CompReadyProgress apiLink={apiCallStr} cancelFunc={cancelFetch} />
  }

  const history = useHistory()

  const handleClick = () => {
    history.push('/component_readiness')
  }
  const columnNames = getColumns(data)
  if (columnNames[0] === 'Cancelled' || columnNames[0] == 'None') {
    return (
      <Fragment>
        <p>Operation cancelled or no data</p>
        <button onClick={handleClick}>Start Over</button>
      </Fragment>
    )
  }

  return (
    <Fragment>
      {pageTitle}
      <h2>
        <Link to="/component_readiness">/</Link> {envDisplay} &gt; {comp}
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
}
