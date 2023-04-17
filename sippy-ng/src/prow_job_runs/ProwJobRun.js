import * as D3 from 'd3'
import { Backdrop, Button, CircularProgress, Tooltip } from '@material-ui/core'
import { BOOKMARKS } from '../constants'
import { Error } from '@material-ui/icons'
import { safeEncodeURIComponent } from '../helpers'
import Alert from '@material-ui/lab/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import TimelinesChart from 'timelines-chart'

export default function ProwJobRun(props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [eventIntervals, setEventIntervals] = React.useState([])

  const fetchData = () => {
    let queryString = ''
    console.log('hello world we got the prow job run id of ' + props.jobRunID)

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/jobs/runs/intervals?prow_job_run_id=' +
        props.jobRunID +
        queryString
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        if (json != null) {
          setEventIntervals(json.items)
        } else {
          setEventIntervals([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve intervals for ' + props.jobRunID + ', ' + error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading intervals for job run {props.jobRunID}...</p>
  }

  return (
    /* eslint-disable react/prop-types */
    <Fragment>
      <p>Loaded {eventIntervals.length} intervals</p>

      <div id="chart"></div>
    </Fragment>
  )
}

ProwJobRun.defaultProps = {}

ProwJobRun.propTypes = {
  jobRunID: PropTypes.string.isRequired,
  filterModel: PropTypes.object,
}
