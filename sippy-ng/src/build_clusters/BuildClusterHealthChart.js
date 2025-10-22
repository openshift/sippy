import { apiFetch } from '../helpers'
import { Line } from 'react-chartjs-2'
import Alert from '@mui/material/Alert'
import chroma from 'chroma-js'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

export default function BuildClusterHealthChart(props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState([])

  const fetchData = () => {
    apiFetch('/api/health/build_cluster/analysis?period=' + props.period)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setData(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve release tag data ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  let periodCount = 14
  if (props.period === 'hour') {
    periodCount = 24
  }

  let lastPeriodsSet = new Set()
  Object.keys(data).forEach((k) => {
    Object.keys(data[k].by_period).forEach((p) => {
      lastPeriodsSet.add(p)
    })
  })
  const lastPeriods = Array.from(lastPeriodsSet)
  lastPeriods.sort()

  const colors = chroma
    .scale('Spectral')
    .mode('lch')
    .colors(Object.keys(data).length)

  const chartData = {
    labels: Array.from(lastPeriods),
    datasets: [],
  }

  const options = {
    plugins: {
      tooltip: {
        callbacks: {
          label: function (context, index) {
            return `${context.dataset.label} ${context.formattedValue}% (${
              data[context.dataset.label].by_period[context.label].current_runs
            } runs)`
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
        grid: {
          z: 1,
        },
        max: 100,
      },
    },
  }

  Object.keys(data).forEach((cluster, index) => {
    chartData.datasets.push({
      label: cluster,
      borderColor: colors[index],
      backgroundColor: colors[index],
      data: Array.from(lastPeriods).map((day) =>
        data[cluster].by_period[day]
          ? data[cluster].by_period[day].current_pass_percentage
          : NaN
      ),
      tension: 0.4,
    })
  })

  return <Line key="build-cluster-chart" options={options} data={chartData} />
}

BuildClusterHealthChart.propTypes = {
  period: PropTypes.string.isRequired,
}
