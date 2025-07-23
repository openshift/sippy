import { Bar } from 'react-chartjs-2'
import { Chart } from 'chart.js'
import { pathForJobsInPercentile } from '../helpers'
import { useHistory } from 'react-router-dom'
import { useTheme } from '@mui/material/styles'
import annotationPlugin from 'chartjs-plugin-annotation'
import chroma from 'chroma-js'
import PropTypes from 'prop-types'
import React from 'react'

Chart.register(annotationPlugin)

export default function Histogram(props) {
  const theme = useTheme()
  const history = useHistory()

  const handleClick = (e) => {
    let percentile = e[0].index * 10
    history.push(
      pathForJobsInPercentile(props.release, percentile, percentile + 10)
    )
  }

  const colors = chroma
    .scale([
      theme.palette.error.light,
      theme.palette.warning.light,
      theme.palette.success.light,
    ])
    .colors(10)

  const chart = {
    labels: [...Array(10).keys()].map(
      (key) => `${key * 10} - ${key * 10 + 10}%`
    ),
    datasets: [
      {
        type: 'bar',
        barPercentage: 1.0,
        categoryPercentage: 1.0,
        backgroundColor: colors,
        data: props.data,
      },
    ],
  }

  const options = {
    plugins: {
      legend: {
        display: false,
      },
      annotation: {
        annotations: [
          {
            type: 'line',
            xMin: props.current_mean / 10,
            xMax: props.current_mean / 10,
            label: {
              enabled: true,
              content: `Current ${props.current_mean.toFixed(0)}%`,
              position: 'start',
              xAdjust: 25,
            },
          },
          {
            type: 'line',
            xMin: props.previous_mean / 10,
            xMax: props.previous_mean / 10,
            borderDash: [5, 6],
            label: {
              enabled: true,
              content: `Previous ${props.previous_mean.toFixed(0)}%`,
              position: 'start',
              yAdjust: 25,
              xAdjust: -25,
            },
          },
        ],
      },
    },
  }

  return <Bar data={chart} options={options} getElementAtEvent={handleClick} />
}

Histogram.propTypes = {
  release: PropTypes.string,
  buckets: PropTypes.number,
  data: PropTypes.array,
  current_mean: PropTypes.number,
  previous_mean: PropTypes.number,
}
