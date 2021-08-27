import { Line } from 'react-chartjs-2'
import Alert from '@material-ui/lab/Alert'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

import './JobAnalysis.css'

export function JobStackedChart(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [analysis, setAnalysis] = React.useState(props.analysis)
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    let queryParams = `release=${props.release}`
    if (props.filter) {
      queryParams += `&filter=${encodeURIComponent(
        JSON.stringify(props.filter)
      )}`
    }

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/jobs/analysis?${queryParams}`
      ),
    ])
      .then(([analysis]) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        return Promise.all([analysis.json()])
      })
      .then(([analysis]) => {
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
      setLoaded(true)
    } else {
      fetchData()
    }
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  const resultChart = {
    labels: Object.keys(analysis.by_day),
    datasets: [],
  }

  const resultTypes = {
    S: {
      color: '#1a9850',
      name: 'Success',
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

  const colorByName = {}
  Object.keys(resultTypes).forEach((key) => {
    colorByName[resultTypes[key].name] = resultTypes[key].color
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
      borderColor: resultTypes[result].color,
      backgroundColor: resultTypes[result].color,
      data: Object.keys(analysis.by_day).map(
        (key) =>
          100 *
          ((analysis.by_day[key].result_count[result] || 0) /
            analysis.by_day[key].total_runs)
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
    },
    scales: {
      x: {
        grid: {
          z: 1,
        },
      },
      y: {
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
    },
  }

  return (
    <Line
      key="result-chart"
      data={resultChart}
      options={options}
      height={100}
    />
  )
}

JobStackedChart.propTypes = {
  release: PropTypes.string.isRequired,
  filter: PropTypes.object,
  analysis: PropTypes.object,
}
