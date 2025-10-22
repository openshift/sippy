import { CircularProgress } from '@mui/material'
import { Line } from 'react-chartjs-2'
import { apiFetch, safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

export function TestDurationChart(props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [durations, setDurations] = React.useState([])

  useEffect(() => {
    fetchData()
  }, [])

  const fetchData = () => {
    let queryString = ''
    if (props.filterModel && props.filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(props.filterModel))
    }

    apiFetch(
      `/api/tests/durations?release=${
          props.release
        }&test=${safeEncodeURIComponent(props.test)}` +
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
          setDurations(json)
        } else {
          setDurations([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve test outputs ' + props.release + ', ' + error
        )
      })
  }

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <CircularProgress color="inherit" />
  }

  const chart = {
    labels: Object.keys(durations),
    datasets: [
      {
        type: 'line',
        label: 'overall',
        tension: 0.25,
        borderColor: 'black',
        backgroundColor: 'black',
        fill: false,
        data: Object.keys(durations).map((key) => durations[key]),
      },
    ],
  }

  return <Line data={chart} height={80} />
}

TestDurationChart.propTypes = {
  release: PropTypes.string.isRequired,
  test: PropTypes.string.isRequired,
  filterModel: PropTypes.object,
}
