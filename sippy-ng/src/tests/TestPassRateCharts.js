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
  const [analysis, setAnalysis] = React.useState({ by_day: {} })
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    const filter = safeEncodeURIComponent(JSON.stringify(props.filterModel))

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/tests/analysis?release=${props.release}&test=${props.test}&filter=${filter}`
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
          'Could not retrieve test analysis ' + props.release + ', ' + error
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
          <Card
            className="test-failure-card"
            elevation={5}
            style={{ minHeight: 80 }}
          >
            <Typography variant="h5">
              Pass Rate By Job
              <Tooltip title="Only jobs with at least one failure over the reporting period are shown individually.">
                <InfoIcon />
              </Tooltip>
            </Typography>
            <CircularProgress color="inherit" />
          </Card>
        </Grid>

        <Grid item md={12}>
          <Card className="test-failure-card" elevation={5}>
            <Typography variant="h5">
              Pass Rate By Variant
              <Tooltip title="Test pass rate is approximated by number of job runs on the given day without a test failure. Only variants with at least one failure over the reporting period are shown individually.">
                <InfoIcon />
              </Tooltip>
            </Typography>
            <CircularProgress color="inherit" />
          </Card>
        </Grid>
      </Fragment>
    )
  }

  const percentage = (passes, runs) => {
    return (100 * (passes / runs)).toFixed(2)
  }

  const byJobChart = {
    labels: Object.keys(analysis.by_day),
    datasets: [
      {
        type: 'line',
        label: 'overall',
        tension: 0.25,
        borderColor: 'black',
        backgroundColor: 'black',
        fill: false,
        data: Object.keys(analysis.by_day).map((key) => {
          const passes = analysis.by_day[key].overall.passes
          const runs = analysis.by_day[key].overall.runs

          return percentage(passes, runs)
        }),
      },
    ],
  }

  const byVariantChart = {
    labels: Object.keys(analysis.by_day),
    datasets: [
      {
        type: 'line',
        label: 'overall',
        tension: 0.25,
        borderColor: 'black',
        backgroundColor: 'black',
        fill: false,
        data: Object.keys(analysis.by_day).map((key) => {
          const passes = analysis.by_day[key].overall.passes
          const runs = analysis.by_day[key].overall.runs

          return percentage(passes, runs)
        }),
      },
    ],
  }

  // Get list of jobs in this data set
  const jobs = new Set()

  // Get a list of variants in this data set
  const variants = new Set()
  const variantFailures = []

  Object.keys(analysis.by_day).forEach((key) => {
    Object.keys(analysis.by_day[key].by_variant || {}).forEach((variant) => {
      variants.add(variant)
    })

    Object.keys(analysis.by_day[key].by_job || {}).forEach((job) => {
      // Omit jobs that never failed
      if (
        analysis.by_day[key].by_job[job].failures > 0 ||
        analysis.by_day[key].by_job[job].flakes > 0
      ) {
        jobs.add(job)
      }
    })
  })

  const colors = scale('Set2').mode('lch').colors(jobs.size)

  const byJobChartOptions = {
    plugins: {
      tooltip: {
        callbacks: {
          label: function (context) {
            let passes, failures, flakes, runs

            if (context.dataset.label === 'overall') {
              passes = analysis.by_day[context.label].overall.passes || 0
              flakes = analysis.by_day[context.label].overall.flakes || 0
              failures = analysis.by_day[context.label].overall.failures || 0
            } else {
              passes = analysis.by_day[context.label].by_job[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_job[context.dataset.label]
                    .passes
                : 0

              failures = analysis.by_day[context.label].by_job[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_job[context.dataset.label]
                    .failures
                : 0

              flakes = analysis.by_day[context.label].by_job[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_job[context.dataset.label]
                    .flakes
                : 0
            }

            return `${context.dataset.label} ${context.raw}% (${passes} passed, ${failures} failed, ${flakes} flaked)`
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

  const byVariantChartOptions = {
    plugins: {
      tooltip: {
        callbacks: {
          label: function (context) {
            let passes, failures, flakes

            if (context.dataset.label === 'overall') {
              passes = analysis.by_day[context.label].overall.passes || 0
              flakes = analysis.by_day[context.label].overall.flakes || 0
              failures = analysis.by_day[context.label].overall.failures || 0
            } else {
              passes = analysis.by_day[context.label].by_variant[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_variant[
                    context.dataset.label
                  ].passes
                : 0

              failures = analysis.by_day[context.label].by_variant[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_variant[
                    context.dataset.label
                  ].failures
                : 0

              flakes = analysis.by_day[context.label].by_variant[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_variant[
                    context.dataset.label
                  ].flakes
                : 0
            }

            return `${context.dataset.label} ${context.raw}% (${passes} passed, ${failures} failed, ${flakes} flaked)`
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
  variants.forEach((variant) => {
    variantFailures.push(
      Object.keys(analysis.by_day)
        .map((key) => {
          return analysis.by_day[key].by_variant[variant]
            ? analysis.by_day[key].by_variant[variant].failures
            : 0
        })
        .reduce((acc, val) => acc + val)
    )

    byVariantChart.datasets.push({
      type: 'line',
      label: `${variant}`,
      tension: 0.25,
      yAxisID: 'y',
      borderColor: colors[index],
      backgroundColor: colors[index],
      data: Object.keys(analysis.by_day).map((key) => {
        const passes = analysis.by_day[key].by_variant[variant]
          ? analysis.by_day[key].by_variant[variant].passes
          : 0
        const runs = analysis.by_day[key].by_variant[variant]
          ? analysis.by_day[key].by_variant[variant].runs
          : 0
        // Percentage of variant runs not exhibiting failure, i.e.
        // an approximation of the pass rate
        return percentage(passes, runs)
      }),
    })

    index++
  })

  index = 0
  jobs.forEach((job) => {
    byJobChart.datasets.push({
      type: 'line',
      label: `${job}`,
      tension: 0.25,
      yAxisID: 'y',
      borderColor: colors[index],
      backgroundColor: colors[index],
      data: Object.keys(analysis.by_day).map((key) => {
        const passes = analysis.by_day[key].by_job[job]
          ? analysis.by_day[key].by_job[job].passes
          : 0
        const runs = analysis.by_day[key].by_job[job]
          ? analysis.by_day[key].by_job[job].runs
          : 0
        // Percentage of job runs not exhibiting failure, i.e.
        // an approximation of the pass rate
        return percentage(passes, runs)
      }),
    })

    index++
  })

  const variantPolar = {
    labels: Array.from(variants),
    datasets: [
      {
        label: '# of Failures',
        data: variantFailures,
        lineTension: 0.4,
        backgroundColor: colors,
        borderWidth: 1,
      },
    ],
  }

  return (
    <Fragment>
      <Grid item md={12}>
        <Card className="test-failure-card" elevation={5}>
          <Typography variant="h5">
            Pass Rate By Job
            <Tooltip title="Only jobs with at least one failure over the reporting period are shown individually.">
              <InfoIcon />
            </Tooltip>
          </Typography>
          <Line data={byJobChart} options={byJobChartOptions} height={80} />
        </Card>
      </Grid>

      <Grid item md={12}>
        <Card className="test-failure-card" elevation={5}>
          <Typography variant="h5">
            Pass Rate By Variant
            <Tooltip title="Test pass rate is approximated by number of job runs on the given day without a test failure. Only variants with at least one failure over the reporting period are shown individually.">
              <InfoIcon />
            </Tooltip>
          </Typography>
          <Line
            data={byVariantChart}
            options={byVariantChartOptions}
            height={80}
          />
        </Card>
      </Grid>
    </Fragment>
  )
}

TestPassRateCharts.defaultProps = {}

TestPassRateCharts.propTypes = {
  release: PropTypes.string.isRequired,
  test: PropTypes.string.isRequired,
  filterModel: PropTypes.object.isRequired,
}
