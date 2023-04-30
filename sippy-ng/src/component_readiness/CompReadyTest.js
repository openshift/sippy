import { CircularProgress } from '@material-ui/core'
import {
  Drawer,
  Grid,
  TableContainer,
  Tooltip,
  Typography,
} from '@material-ui/core'
import {
  getAPIUrl,
  getColumns,
  makeRFC3339Time,
  singleRowReport,
} from './CompReadyUtils'
import { Link } from 'react-router-dom'
import Button from '@material-ui/core/Button'
import CompReadyRow from './CompReadyRow'
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
  console.log('Aborting page2')
  abortController.abort()
}

export default function CompReadyCapabilities(props) {
  const { filterVals } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  // Set the browser tab title
  document.title = `CompRead Test`
  const urlParams = new URLSearchParams(location.search)
  const comp = urlParams.get('component')
  const env = urlParams.get('environment')
  console.log('filterVals T: ', filterVals)

  let envStr = '&environment=' + env
  if (filterVals.includes('environment')) {
    let envStr = ''
  }
  const apiCallStr = getAPIUrl() + makeRFC3339Time(filterVals + envStr)

  const noDataTable = {
    rows: [
      {
        component: 'No Data found',
        capability: 'Not setup to get data',
      },
    ],
  }

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
          console.log('Got good page2 json', json)
          if (Object.keys(json).length === 0 || json.rows.length === 0) {
            // The api call returned 200 OK but the data was empty
            setData(noDataTable)
          } else {
            setData(json)
          }
        })
        .catch((error) => {
          if (error.name === 'AbortError') {
            console.log('Request was cancelled')

            // Once this fired, we need a new one for the next button click.
            abortController = new AbortController()
          } else {
            setFetchError(`API call failed: ${formattedApiCallStr}` + error)
          }
        })
        .finally(() => {
          // Mark the attempt as finished whether successful or not.
          setIsLoaded(true)
        })
    }
  }, [])

  console.log('isLoaded page2: ', isLoaded)
  if (!isLoaded) {
    return (
      <Fragment>
        Loading component readiness data ... If you asked for a huge dataset, it
        may take minutes.
        <br />
        Here is the API call in case you are interested:
        <br />
        <h3>
          <a href={apiCallStr}>{apiCallStr}</a>
        </h3>
        <CircularProgress />
        <div>
          <Button
            size="medium"
            variant="contained"
            color="secondary"
            onClick={cancelFetch}
          >
            Cancel
          </Button>
        </div>
      </Fragment>
    )
  }
  console.log('came here: ', data.rows.length, data.rows)
  const columnNames = getColumns(data)
  console.log('columnNames page2:', columnNames)

  return (
    <Fragment>
      <Typography variant="h4" style={{ margin: 20, textAlign: 'center' }}>
        Capabilities report for environment {env}, component {comp}
      </Typography>
      <h2>
        <Link to="/component_readiness">/</Link> {env} &gt; {comp}
      </h2>
      <br></br>
      <TableContainer component="div" className="cr-wrapper">
        <Table className="cr-comp-read-table">
          <TableHead>
            <TableRow>
              <TableCell className={'cr-col-result-full'}>
                <Typography className="cr-cell-name">Name</Typography>
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
                          <Link to={singleRowReport(column)}>{column}</Link>
                        </Typography>
                      </Tooltip>
                    </TableCell>
                  )
                }
              })}
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.keys(data.rows).map((componentIndex) => (
              <CompReadyRow
                key={componentIndex}
                componentName={data.rows[componentIndex].capability}
                results={data.rows[componentIndex].columns}
                columnNames={columnNames}
                filterVals="none"
              />
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Fragment>
  )
}

CompReadyCapabilities.propTypes = {
  filterVals: PropTypes.string.isRequired,
}
