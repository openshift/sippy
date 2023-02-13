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

  let days = Array.from(
    { length: 14 },
    (_, x) => new Date(new Date() - 1000 * 60 * 60 * 24 * x)
  )
    .map((day) => day.toISOString().split('T')[0])
    .reverse()

  const chart = {
    labels: days,
    datasets: [],
  }

  const options = {
    parsing: {
      xAxisKey: 'date',
      yAxisKey: 'pass_percentage',
    },
    plugins: {
      tooltip: {
        callbacks: {
          label: function (context) {
            let data = groupedData[context.dataset.label].find(
              (element) => element.date === context.label
            )

            return `${context.dataset.label} ${data.pass_percentage}% (${data.passes} passed, ${data.failures} failed, ${data.flakes} flaked)`
          },
        },
      },
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
  Object.keys(groupedData).forEach((group) => {
    chart.datasets.push({
      type: 'line',
      label: `${group}`,
      tension: 0.25,
      yAxisID: 'y',
      borderColor: colors[index],
      backgroundColor: colors[index],
      data: groupedData[group],
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
