import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import './TestTable.css'
import { StringParam, useQueryParam } from 'use-query-params'
import Alert from '@material-ui/lab/Alert'
import { Line, PolarArea } from 'react-chartjs-2'
import { scale } from 'chroma-js'
import { Button, Card, Container, Grid, Tooltip, Typography } from '@material-ui/core'
import './TestAnalysis.css'
import Divider from '@material-ui/core/Divider'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import { Link } from 'react-router-dom'
import JobRunsTable from '../jobs/JobRunsTable'
import { filterFor, pathForJobRunsWithTestFailure, searchCI, withSort } from '../helpers'
import BugTable from '../bugzilla/BugTable'
import bugzillaURL from '../bugzilla/BugzillaUtils'
import InfoIcon from '@material-ui/icons/Info'
import { ASSOCIATED_BUGS, LINKED_BUGS } from '../bugzilla/BugzillaDialog'
import { TEST_THRESHOLDS } from '../constants'
import SummaryCard from '../components/SummaryCard'
import PassRateIcon from '../components/PassRateIcon'

export function TestAnalysis (props) {
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

    const filter = encodeURIComponent(JSON.stringify({
      items: [
        filterFor('name', 'equals', testName)
      ]
    }))

    Promise.all([
      fetch(`${process.env.REACT_APP_API_URL}/api/tests/analysis?release=${props.release}&test=${testName}`),
      fetch(`${process.env.REACT_APP_API_URL}/api/tests?release=${props.release}&filter=${filter}`)
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
      }
      ).catch(error => {
        setFetchError('Could not retrieve test failures ' + props.release + ', ' + error)
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
      <p>Loading...</p>
    )
  }

  const chart = {
    labels: Object.keys(analysis.by_day),
    datasets: [
      {
        type: 'line',
        label: 'overall',
        tension: 0.25,
        borderColor: 'black',
        backgroundColor: 'black',
        fill: false,
        data: Object.keys(analysis.by_day).map((key) => (100 * (1 - (analysis.by_day[key].failure_count / analysis.by_day[key].runs)).toFixed(2)))
      }
    ]
  }

  // Get list of variants in this data set
  const variants = new Set()
  const variantFailures = []

  Object.keys(analysis.by_day).forEach((key) => {
    Object.keys(analysis.by_day[key].failure_by_variant || {}).forEach((variant) => {
      variants.add(variant)
    })
  })

  const colors = scale('Set2').mode('lch').colors(variants.size)

  const options = {
    scales: {
      y: {
        ticks: {
          callback: (value, index, values) => {
            return `${value}%`
          }
        }
      }
    }
  }

  let index = 0
  variants.forEach((variant) => {
    variantFailures.push(Object.keys(analysis.by_day).map((key) => {
      return analysis.by_day[key].failure_by_variant[variant] || 0
    }).reduce((acc, val) => acc + val))

    chart.datasets.push({
      type: 'line',
      label: `${variant}`,
      tension: 0.25,
      yAxisID: 'y',
      borderColor: colors[index],
      backgroundColor: colors[index],
      data: Object.keys(analysis.by_day).map((key) => 100 * (1 - ((analysis.by_day[key].failure_by_variant[variant] || 0) / analysis.by_day[key].runs_by_variant[variant])))
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
        borderWidth: 1
      }
    ]
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={
          <Link to={'/tests/' + props.release}>
            Tests
          </Link>
        }
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
            <Card className="test-failure-card" elevation={5} style={{ height: '100%' }}>
              <Typography variant="h5">
                {testName}
              </Typography>
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
                  to={withSort(pathForJobRunsWithTestFailure(props.release, testName), 'timestamp', 'desc')}
                >
                  See all job runs
                </Button>
              </div>
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5}>
              <Typography variant="h5">
                Pass rate (jobs without test failure)
              </Typography>
              <Line data={chart} options={options} height={80} />
            </Card>
          </Grid>

          <Grid item md={4}>
            <Card className="test-failure-card" elevation={5} style={{ height: '100%' }}>
              <Typography variant="h5">
                Failure count by variant
              </Typography>
              <PolarArea data={variantPolar} />
            </Card>
          </Grid>

          <Grid item md={8}>
            <Card className="test-failure-card" elevation={5} style={{ height: '100%' }}>
              <Typography
                component={Link}
                to={withSort(pathForJobRunsWithTestFailure(props.release, testName), 'timestamp', 'desc')}
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
                      columnField: 'failedTestNames',
                      operatorValue: 'contains',
                      value: testName
                    }
                  ]
                }}
              />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5} style={{ height: '100%' }}>
              <Typography>Linked bugs <Tooltip title={LINKED_BUGS}><InfoIcon /></Tooltip></Typography>
              <BugTable bugs={test.bugs} />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5} style={{ height: '100%' }}>
              <Typography>Associated bugs <Tooltip title={ASSOCIATED_BUGS}><InfoIcon /></Tooltip></Typography>
              <BugTable bugs={test.associated_bugs} />
            </Card>
          </Grid>

        </Grid>
      </Container>
    </Fragment>
  )
}

TestAnalysis.defaultProps = {
  test: ''
}

TestAnalysis.propTypes = {
  release: PropTypes.string.isRequired,
  test: PropTypes.string
}
