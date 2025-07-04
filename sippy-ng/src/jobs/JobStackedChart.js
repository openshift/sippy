import { Line } from 'react-chartjs-2'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

import './JobAnalysis.css'
import { safeEncodeURIComponent } from '../helpers'
import { useNavigate } from 'react-router-dom'

export const dayFilter = (days, startDate) => {
  return [
    {
      columnField: 'timestamp',
      operatorValue: '>',
      value: `${startDate - 1000 * 60 * 60 * 24 * days}`,
    },
  ]
}

export const hourFilter = (dayOffset, startDate) => {
  return [
    {
      columnField: 'timestamp',
      operatorValue: '>',
      value: `${startDate - dayOffset * 1000 * 60 * 60 * 24}`,
    },
    {
      columnField: 'timestamp',
      operatorValue: '<=',
      value: `${startDate - (dayOffset - 1) * 1000 * 60 * 60 * 24}`,
    },
  ]
}

export function JobStackedChart(props) {
  const navigate = useNavigate()

  const [isLoaded, setLoaded] = React.useState(false)
  const [analysis, setAnalysis] = React.useState({})
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    let queryParams = `release=${props.release}`
    if (props.filter) {
      queryParams += `&filter=${safeEncodeURIComponent(
        JSON.stringify(props.filter)
      )}`
    }

    if (props.period) {
      queryParams += `&period=${props.period}`
    }

    fetch(`${process.env.REACT_APP_API_URL}/api/jobs/analysis?${queryParams}`)
      .then((analysis) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        return analysis.json()
      })
      .then((analysis) => {
        setAnalysis(analysis)
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
    return <p>Loading</p>
  }

  const resultChart = {
    labels: Object.keys(analysis.by_period),
    datasets: [],
  }

  const resultTypes = {
    S: {
      color: '#1a9850',
      name: 'Success',
    },
    A: {
      color: '#707070',
      name: 'Aborted',
    },
    R: {
      color: '#aaa',
      name: 'Running',
    },
    N: {
      color: '#fdae61',
      name: 'Setup failure (infra)',
    },
    I: {
      color: '#fee08b',
      name: 'Setup failure (installer)',
    },
    U: {
      color: '#ff6961',
      name: 'Upgrade failure',
    },
    F: {
      color: '#d73027',
      name: 'Failure (e2e)',
    },
    n: {
      color: '#633',
      name: 'Failure before setup',
    },
    f: {
      color: '#f46d43',
      name: 'Failure (other)',
    },
  }

  const handleClick = (e) => {
    navigate(
      `/jobs/${
        props.release
      }/analysis?period=day&filters=${safeEncodeURIComponent(
        JSON.stringify(props.filter)
      )}`
    )
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
    data: Object.keys(analysis.by_period).map(
      (key) => analysis.by_period[key].total_runs
    ),
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
      data: Object.keys(analysis.by_period).map(
        (key) =>
          100 *
          ((analysis.by_period[key].result_count[result] || 0) /
            analysis.by_period[key].total_runs)
      ),
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
      tooltip: {
        callbacks: {
          label: function (context) {
            if (context.dataset.label === 'Run count') {
              return `${context.formattedValue} total runs`
            }

            return `${context.formattedValue}% (${
              analysis.by_period[context.label].total_runs
            } total runs)`
          },
        },
      },
    },
    scales: {
      x: {
        grid: {
          z: 1,
        },
      },
      y: {
        suggestedMin: 0,
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
    <Line
      key="result-chart"
      data={resultChart}
      options={options}
      height={120}
      getElementAtEvent={handleClick}
    />
  )
}

JobStackedChart.propTypes = {
  release: PropTypes.string.isRequired,
  filter: PropTypes.object,
  period: PropTypes.string,
  analysis: PropTypes.object,
}
