import './TestAnalysis.css'
import './TestTable.css'
import { BugReport, DirectionsRun } from '@material-ui/icons'
import {
  Button,
  Card,
  Container,
  Grid,
  Tooltip,
  Typography,
} from '@material-ui/core'
import {
  filterFor,
  not,
  pathForJobRunsWithTestFailure,
  safeEncodeURIComponent,
  SafeJSONParam,
  SafeStringParam,
  searchCI,
  withSort,
} from '../helpers'
import { Line, PolarArea } from 'react-chartjs-2'
import { Link } from 'react-router-dom'
import { scale } from 'chroma-js'
import { TEST_THRESHOLDS } from '../constants'
import { useQueryParam } from 'use-query-params'
import Alert from '@material-ui/lab/Alert'
import bugzillaURL from '../bugzilla/BugzillaUtils'
import Divider from '@material-ui/core/Divider'
import GridToolbarFilterMenu from '../datagrid/GridToolbarFilterMenu'
import InfoIcon from '@material-ui/icons/Info'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import SummaryCard from '../components/SummaryCard'
import TestTable from './TestTable'

export function TestAnalysis(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [analysis, setAnalysis] = React.useState({ by_day: {} })
  const [test, setTest] = React.useState({})
  const [fetchError, setFetchError] = React.useState('')
  const [testName = props.test] = useQueryParam('test', SafeStringParam)

  const [
    filterModel = {
      items: [
        filterFor('name', 'equals', testName),
        not(filterFor('variants', 'contains', 'never-stable')),
      ],
    },
    setFilterModel,
  ] = useQueryParam('filters', SafeJSONParam)

  const setFilterModelSafe = (m) => {
    setFilterModel(m)
    fetchData()
  }

  const fetchData = () => {
    document.title = `Sippy > ${props.release} > Tests > ${testName}`
    if (!testName || testName === '') {
      setFetchError('Test name is required.')
      return
    }

    const filter = safeEncodeURIComponent(JSON.stringify(filterModel))

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/tests/analysis?release=${props.release}&test=${testName}&filter=${filter}`
      ),
      fetch(
        `${process.env.REACT_APP_API_URL}/api/tests?release=${props.release}&filter=${filter}`
      ),
    ])
      .then(([analysis, test]) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        if (test.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }
        return Promise.all([analysis.json(), test.json()])
      })
      .then(([failures, test]) => {
        if (test.length === 0) {
          return <Typography variant="h5">No data for this test.</Typography>
        }

        setAnalysis(failures)
        setTest(test[0])
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve test failures ' + props.release + ', ' + error
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
    return <p>Loading...</p>
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
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={<Link to={'/tests/' + props.release}>Tests</Link>}
        currentPage="Test Analysis"
      />

      <Container maxWidth="xl">
        <Typography variant="h3" style={{ textAlign: 'center' }}>
          Test Analysis
        </Typography>
        <Divider style={{ margin: 20 }} />

        <Grid container spacing={3} alignItems="stretch">
          <Grid item md={3}>
            <SummaryCard
              key="test-summary"
              threshold={TEST_THRESHOLDS}
              name="Overall"
              success={test.current_successes}
              flakes={test.current_flakes}
              caption={
                <Fragment>
                  <Tooltip title={`${test.current_runs} runs`}>
                    <span>{test.current_working_percentage.toFixed(2)}%</span>
                  </Tooltip>
                  <PassRateIcon improvement={test.net_improvement} />
                  <Tooltip title={`${test.previous_runs} runs`}>
                    <span>{test.previous_working_percentage.toFixed(2)}%</span>
                  </Tooltip>
                </Fragment>
              }
              fail={test.current_failures}
            />
          </Grid>
          <Grid item md={3}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <Typography variant="h5">Failure count by variant</Typography>
              <PolarArea data={variantPolar} />
            </Card>
          </Grid>
          <Grid item md={6}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <Typography variant="h5" style={{ paddingBottom: 20 }}>
                {testName}
              </Typography>

              <Grid container justifyContent="space-between">
                <GridToolbarFilterMenu
                  linkOperatorDisabled={true}
                  standalone={true}
                  filterModel={filterModel || { items: [] }}
                  setFilterModel={setFilterModelSafe}
                  columns={[
                    {
                      field: 'name',
                      headerName: 'Name',
                      filterable: true,
                      disabled: true,
                      type: 'string',
                    },
                    {
                      field: 'variants',
                      headerName: 'Variants',
                      filterable: true,
                      autocomplete: 'variants',
                      type: 'array',
                    },
                  ]}
                />

                <Button
                  className="test-button"
                  target="_blank"
                  startIcon={<BugReport />}
                  variant="contained"
                  color="primary"
                  href={bugzillaURL(props.release, test)}
                >
                  Open bug
                </Button>

                <Button
                  className="test-button"
                  target="_blank"
                  startIcon={<InfoIcon />}
                  variant="contained"
                  color="secondary"
                  href={searchCI(testName)}
                >
                  Search Logs
                </Button>

                <Button
                  className="test-button"
                  variant="contained"
                  startIcon={<DirectionsRun />}
                  component={Link}
                  to={withSort(
                    pathForJobRunsWithTestFailure(props.release, testName, {
                      items: [
                        ...filterModel.items.filter(
                          (f) => f.columnField === 'variants'
                        ),
                      ],
                    }),
                    'timestamp',
                    'desc'
                  )}
                >
                  See job runs
                </Button>
              </Grid>
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5}>
              <Typography variant="h5">
                Pass Rate By NURP+ Combination
                <Tooltip
                  title={
                    <p>
                      NURP+ is the combination of a job&apos;s <b>N</b>etwork
                      (e.g. sdn, ovn), <b>U</b>pgrade, from <b>R</b>elease (e.g.
                      upgrading from a minor or micro),
                      <b>P</b>latform (aws, azure, etc) and extras (realtime,
                      serial, etc). It shows the current 7 day period compared
                      to the last 7 day period by default.
                    </p>
                  }
                >
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <TestTable
                pageSize={5}
                hideControls={true}
                collapse={false}
                release={props.release}
                filterModel={filterModel}
              />
            </Card>
          </Grid>

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
        </Grid>
      </Container>
    </Fragment>
  )
}

TestAnalysis.defaultProps = {
  test: '',
}

TestAnalysis.propTypes = {
  release: PropTypes.string.isRequired,
  test: PropTypes.string,
}
