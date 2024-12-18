import { Card, CircularProgress, Grid, Typography } from '@mui/material'
import { Line } from 'react-chartjs-2'
import { safeEncodeURIComponent } from '../helpers'
import { useTheme } from '@mui/material/styles'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

export function TestStackedChart(props) {
  const theme = useTheme()

  const [isLoaded, setLoaded] = React.useState(false)
  const [analysis, setAnalysis] = React.useState([])
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    let queryParams = `release=${props.release}&test=${safeEncodeURIComponent(
      props.test
    )}`

    if (props.filter) {
      queryParams += `&filter=${safeEncodeURIComponent(
        JSON.stringify(props.filter)
      )}`
    }

    fetch(
      `${process.env.REACT_APP_API_URL}/api/tests/analysis/overall?${queryParams}`
    )
      .then((analysis) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        return analysis.json()
      })
      .then((analysis) => {
        setAnalysis(analysis['overall'])
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve job analysis ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    if (props.analysis) {
      setAnalysis(props.analysis)
      setLoaded(true)
    } else {
      fetchData()
    }
  }, [props])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return (
      <Fragment>
        <Grid item md={12}>
          <Card className="test-failure-card" elevation={5}>
            <Typography variant="h5">Overall Results</Typography>
            <CircularProgress color="inherit" />
          </Card>
        </Grid>
      </Fragment>
    )
  }

  if (analysis == null) {
    return (
      <Fragment>
        <Grid item md={12}>
          <Card className="test-failure-card" elevation={5}>
            <Typography variant="h5">Overall Results</Typography>
            <Typography>No analysis available for this test.</Typography>
          </Card>
        </Grid>
      </Fragment>
    )
  }

  let daySet = new Set()
  analysis.forEach((dt) => {
    daySet.add(dt.date)
  })
  const resultChart = {
    labels: Array.from(daySet),
    datasets: [],
  }

  const resultTypes = {
    S: {
      color: theme.palette.success.main,
      key: 'pass_percentage',
      name: 'Success',
    },
    F: {
      color: theme.palette.warning.main,
      key: 'flake_percentage',
      name: 'Flake',
    },
    E: {
      color: theme.palette.error.main,
      key: 'fail_percentage',
      name: 'Fail',
    },
  }

  const colorByName = {}
  Object.keys(resultTypes).forEach((key) => {
    colorByName[resultTypes[key].name] = resultTypes[key].color
  })

  resultChart.datasets.push({
    label: 'Run count',
    tension: 0.5,
    radius: 2,
    yAxisID: 'y1',
    xAxisID: 'x',
    order: 1,
    borderColor: '#000000',
    backgroundColor: '#000000',
    data: analysis.map((day) => day.runs),
  })

  Object.keys(resultTypes).forEach((result) => {
    resultChart.datasets.push({
      type: 'line',
      fill: 'origin',
      radius: 1,
      label: `${resultTypes[result].name}`,
      tension: 0.3,
      yAxisID: 'y',
      xAxisID: 'x',
      order: 2,
      borderColor: resultTypes[result].color,
      backgroundColor: resultTypes[result].color,
      data: analysis.map((day) => day[resultTypes[result].key]),
    })
  })

  const handleHover = (e, item, legend) => {
    legend.chart.data.datasets.forEach((dataset, index) => {
      if (index !== item.datasetIndex) {
        dataset.backgroundColor = '#ffffff'
        dataset.borderColor = '#ffffff'
      }
    })
    legend.chart.update()
  }

  const handleLeave = (e, item, legend) => {
    legend.chart.data.datasets.forEach((dataset, index) => {
      dataset.backgroundColor = colorByName[dataset.label]
      dataset.borderColor = colorByName[dataset.label]
    })
    legend.chart.update()
  }

  const options = {
    plugins: {
      line: {
        onHover: handleHover,
        onLeave: handleLeave,
      },
    },
    scales: {
      x: {
        grid: {
          z: 1,
        },
      },
      y: {
        suggestedMax: 80,
        grid: {
          z: 1,
        },
        stacked: true,
        max: 100,
        ticks: {
          callback: (value, index, values) => {
            return `${value}%`
          },
        },
      },
      y1: {
        type: 'linear',
        display: true,
        position: 'right',
      },
    },
  }

  return (
    <Fragment>
      <Grid item md={12}>
        <Card className="test-failure-card" elevation={5}>
          <Typography variant="h5">Overall Results</Typography>
          <Line
            key="result-chart"
            data={resultChart}
            options={options}
            height={80}
          />
        </Card>
      </Grid>
    </Fragment>
  )
}

TestStackedChart.propTypes = {
  release: PropTypes.string.isRequired,
  test: PropTypes.string.isRequired,
  filter: PropTypes.object,
  period: PropTypes.string,
  analysis: PropTypes.array,
}
