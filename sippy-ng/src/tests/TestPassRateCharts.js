import {
  Card,
  CircularProgress,
  Grid,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { Line } from 'react-chartjs-2'
import { safeEncodeURIComponent } from '../helpers'
import { scale } from 'chroma-js'
import Alert from '@material-ui/lab/Alert'
import InfoIcon from '@material-ui/icons/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

export default function TestPassRateCharts(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [groupedData, setGroupedData] = React.useState({})
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    const filter = safeEncodeURIComponent(JSON.stringify(props.filterModel))

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/tests/analysis/${props.grouping}?release=${props.release}&test=${props.test}&filter=${filter}`
      ),
    ])
      .then(([apiResponse]) => {
        if (apiResponse.status !== 200) {
          throw new Error('server returned ' + apiResponse.status)
        }
        return Promise.all([apiResponse.json()])
      })
      .then(([apiResponse]) => {
        setGroupedData(apiResponse)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve test analysis for ' +
            props.release +
            +' ' +
            props.grouping +
            ' , ' +
            error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return (
      <Fragment>
        <Grid item md={12}>
          <Card className="test-failure-card" elevation={5}>
            <Typography variant="h5">Pass Rate By {props.grouping}</Typography>
            <CircularProgress color="inherit" />
          </Card>
        </Grid>
      </Fragment>
    )
  }

  const colors = scale('Set2')
    .mode('lch')
    .colors(Object.keys(groupedData).length)

  const chart = { datasets: [] }
  const options = {
    parsing: {
      xAxisKey: 'date',
      yAxisKey: 'pass_percentage',
    },
    scales: {
      y: {
        max: 100,
        ticks: {
          callback: (value, index, values) => {
            return `${value}%`
          },
        },
      },
    },
  }

  let index = 0
  Object.keys(groupedData).forEach((variant) => {
    chart.datasets.push({
      type: 'line',
      label: `${variant}`,
      tension: 0.25,
      yAxisID: 'y',
      borderColor: colors[index],
      backgroundColor: colors[index],
      data: groupedData[variant],
    })
    index++
  })

  return (
    <Fragment>
      <Grid item md={12}>
        <Card className="test-failure-card" elevation={5}>
          <Typography variant="h5">
            Pass Rate By{' '}
            {props.grouping.charAt(0).toUpperCase() +
              props.grouping.substr(1).toLowerCase()}
          </Typography>
          <Line data={chart} options={options} height={80} />
        </Card>
      </Grid>
    </Fragment>
  )
}

TestPassRateCharts.defaultProps = {
  grouping: 'variants',
}

TestPassRateCharts.propTypes = {
  grouping: PropTypes.string,
  release: PropTypes.string.isRequired,
  test: PropTypes.string.isRequired,
  filterModel: PropTypes.object.isRequired,
}
