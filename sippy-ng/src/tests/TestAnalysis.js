import './TestAnalysis.css'
import './TestTable.css'
import { ASSOCIATED_BUGS, LINKED_BUGS } from '../bugzilla/BugzillaDialog'
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
  pathForJobRunsWithTestFailure,
  searchCI,
  withSort,
} from '../helpers'
import { Line, PolarArea } from 'react-chartjs-2'
import { Link } from 'react-router-dom'
import { scale } from 'chroma-js'
import { StringParam, useQueryParam } from 'use-query-params'
import { TEST_THRESHOLDS } from '../constants'
import Alert from '@material-ui/lab/Alert'
import BugTable from '../bugzilla/BugTable'
import bugzillaURL from '../bugzilla/BugzillaUtils'
import Divider from '@material-ui/core/Divider'
import InfoIcon from '@material-ui/icons/Info'
import JobRunsTable from '../jobs/JobRunsTable'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import SummaryCard from '../components/SummaryCard'

export function TestAnalysis(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [analysis, setAnalysis] = React.useState({ by_day: {} })
  const [test, setTest] = React.useState({})
  const [fetchError, setFetchError] = React.useState('')

  const [testName = props.test] = useQueryParam('test', StringParam)

  const fetchData = () => {
    document.title = `Sippy > ${props.release} > Tests > ${testName}`
    if (!testName || testName === '') {
      setFetchError('Test name is required.')
      return
    }

    const filter = encodeURIComponent(
      JSON.stringify({
        items: [filterFor('name', 'equals', testName)],
      })
    )

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/tests/analysis?release=${props.release}&test=${testName}`
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

  // An approximation of the pass rate is number of runs that presented
  // without failure.
  const inversePercentage = (failures, runs) => {
    return (100 * (1 - failures / runs)).toFixed(2)
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
          const failures = analysis.by_day[key].overall.failures
          const runs = analysis.by_day[key].overall.runs

          return inversePercentage(failures, runs)
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
          const failures = analysis.by_day[key].overall.failures
          const runs = analysis.by_day[key].overall.runs

          return inversePercentage(failures, runs)
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
      if (analysis.by_day[key].by_variant[variant].failures !== 0) {
        variants.add(variant)
      }
    })

    Object.keys(analysis.by_day[key].by_job || {}).forEach((job) => {
      // Omit jobs that never failed
      if (analysis.by_day[key].by_job[job].failures !== 0) {
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
            let failures, runs

            if (context.dataset.label === 'overall') {
              failures = analysis.by_day[context.label].overall.failures || 0
              runs = analysis.by_day[context.label].overall.runs
            } else {
              failures = analysis.by_day[context.label].by_job[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_job[context.dataset.label]
                    .failures
                : 0
              runs = analysis.by_day[context.label].by_job[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_job[context.dataset.label]
                    .runs
                : 0
            }

            return `${context.dataset.label} ${context.raw}% (${failures}/${runs} runs failed)`
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
            let failures, runs

            if (context.dataset.label === 'overall') {
              failures = analysis.by_day[context.label].overall.failures || 0
              runs = analysis.by_day[context.label].overall.runs
            } else {
              failures = analysis.by_day[context.label].by_variant[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_variant[
                    context.dataset.label
                  ].failures
                : 0
              runs = analysis.by_day[context.label].by_variant[
                context.dataset.label
              ]
                ? analysis.by_day[context.label].by_variant[
                    context.dataset.label
                  ].runs
                : 0
            }

            return `${context.dataset.label} ${context.raw}% (${failures}/${runs} runs failed)`
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
        const failures = analysis.by_day[key].by_variant[variant]
          ? analysis.by_day[key].by_variant[variant].failures
          : 0
        const runs = analysis.by_day[key].by_variant[variant]
          ? analysis.by_day[key].by_variant[variant].runs
          : 0
        // Percentage of variant runs not exhibiting failure, i.e.
        // an approximation of the pass rate
        return inversePercentage(failures, runs)
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
        const failures = analysis.by_day[key].by_job[job]
          ? analysis.by_day[key].by_job[job].failures
          : 0
        const runs = analysis.by_day[key].by_job[job]
          ? analysis.by_day[key].by_job[job].runs
          : 0
        // Percentage of job runs not exhibiting failure, i.e.
        // an approximation of the pass rate
        return inversePercentage(failures, runs)
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

      <Container size="xl">
        <Typography variant="h3" style={{ textAlign: 'center' }}>
          Test Analysis
        </Typography>
        <Divider style={{ margin: 20 }} />

        <Grid container spacing={3} alignItems="stretch">
          <Grid item md={4}>
            <SummaryCard
              key="test-summary"
              threshold={TEST_THRESHOLDS}
              name="Overall"
              success={test.current_successes}
              flakes={test.current_flakes}
              caption={
                <Fragment>
                  <Tooltip title={`${test.current_runs} runs`}>
                    <span>{test.current_pass_percentage.toFixed(2)}%</span>
                  </Tooltip>
                  <PassRateIcon improvement={test.net_improvement} />
                  <Tooltip title={`${test.previous_runs} runs`}>
                    <span>{test.previous_pass_percentage.toFixed(2)}%</span>
                  </Tooltip>
                </Fragment>
              }
              fail={test.current_failures}
            />
          </Grid>

          <Grid item md={8}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <Typography variant="h5">{testName}</Typography>
              <Divider className="test-divider" />
              <div align="center">
                <Button
                  className="test-button"
                  target="_blank"
                  variant="contained"
                  color="primary"
                  href={bugzillaURL(props.release, test)}
                >
                  Open a new bug
                </Button>

                <Button
                  className="test-button"
                  target="_blank"
                  variant="contained"
                  color="secondary"
                  href={searchCI(testName)}
                >
                  Search CI Logs
                </Button>

                <Button
                  className="test-button"
                  variant="contained"
                  component={Link}
                  to={withSort(
                    pathForJobRunsWithTestFailure(props.release, testName),
                    'timestamp',
                    'desc'
                  )}
                >
                  See all job runs
                </Button>
              </div>
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5}>
              <Typography variant="h5">
                Pass Rate By Job{' '}
                <Tooltip title="Test pass rate is approximated by number of job runs on the given day without a test failure. Only jobs with at least one failure over the reporting period are shown individually.">
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <Line data={byJobChart} options={byJobChartOptions} height={80} />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5}>
              <Typography variant="h5">
                Pass Rate By Variant{' '}
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

          <Grid item md={4}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <Typography variant="h5">Failure count by variant</Typography>
              <PolarArea data={variantPolar} />
            </Card>
          </Grid>

          <Grid item md={8}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <Typography
                component={Link}
                to={withSort(
                  pathForJobRunsWithTestFailure(props.release, testName),
                  'timestamp',
                  'desc'
                )}
                variant="h5"
                style={{ marginBottom: 20 }}
              >
                Job runs with test failure
              </Typography>

              <JobRunsTable
                hideControls={true}
                release={props.release}
                briefTable={true}
                pageSize={5}
                sortField="timestamp"
                sort="desc"
                filterModel={{
                  items: [
                    {
                      id: 99,
                      columnField: 'failed_test_names',
                      operatorValue: 'contains',
                      value: testName,
                    },
                  ],
                }}
              />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <Typography>
                Linked bugs{' '}
                <Tooltip title={LINKED_BUGS}>
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <BugTable bugs={test.bugs} />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <Typography>
                Associated bugs{' '}
                <Tooltip title={ASSOCIATED_BUGS}>
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <BugTable bugs={test.associated_bugs} />
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
