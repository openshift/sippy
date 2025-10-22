import './TestAnalysis.css'
import './TestTable.css'
import {
  Box,
  Button,
  Card,
  Container,
  Grid,
  TableContainer,
  Tooltip,
  Typography,
} from '@mui/material'
import { DirectionsRun } from '@mui/icons-material'
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
import { Link } from 'react-router-dom'
import { TEST_THRESHOLDS } from '../constants'
import { TestDurationChart } from './TestDurationChart'
import { TestOutputs } from './TestOutputs'
import { TestStackedChart } from './TestStackedChart'
import { usePageContextForChat } from '../chat/store/useChatStore'
import { useQueryParam } from 'use-query-params'
import Alert from '@mui/material/Alert'
import BugButton from '../bugs/BugButton'
import BugTable from '../bugs/BugTable'
import Divider from '@mui/material/Divider'
import GridToolbarFilterMenu from '../datagrid/GridToolbarFilterMenu'
import InfoIcon from '@mui/icons-material/Info'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import SummaryCard from '../components/SummaryCard'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'
import TestPassRateCharts from './TestPassRateCharts'
import TestTable from './TestTable'

export function TestAnalysis(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [test, setTest] = React.useState({})
  const [fetchError, setFetchError] = React.useState('')
  const [testName = props.test] = useQueryParam('test', SafeStringParam)
  const { setPageContextForChat, unsetPageContextForChat } =
    usePageContextForChat()

  const [
    filterModel = {
      items: [
        filterFor('name', 'equals', testName),
        not(filterFor('variants', 'contains', 'aggregated')),
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
        `${process.env.REACT_APP_API_URL}/api/tests?release=${props.release}&filter=${filter}`
      ),
    ])
      .then(([test]) => {
        if (test.status !== 200) {
          throw new Error('server returned ' + test.status)
        }

        return Promise.all([test.json()])
      })
      .then(([test]) => {
        if (test.length === 0) {
          throw new Error('test not found')
        }
        setTest(test[0])
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

  // Update page context for chat
  useEffect(() => {
    if (!isLoaded || !test || !testName) return

    setPageContextForChat({
      page: 'test-analysis',
      url: window.location.href,
      instructions: `The user is viewing detailed analysis for a specific test. 
        You can use your database query tools to answer additional questions about this test.
        When querying the database, use the test name and apply the same filters shown in the context.
        The test statistics shown are for the current 7-day period compared to the previous 7-day period.`,
      suggestions: [
        'What are the most common failure modes for this test?',
        {
          prompt: 'test-analysis',
          label: 'Detailed Test Analysis',
          args: {
            release: props.release,
            test_name: testName,
          },
        },
      ],
      data: {
        release: props.release,
        test_name: testName,
        filters: filterModel,
        statistics: {
          current_pass_percentage: test.current_pass_percentage,
          current_runs: test.current_runs,
          current_successes: test.current_successes,
          current_failures: test.current_failures,
          current_flakes: test.current_flakes,
          previous_pass_percentage: test.previous_pass_percentage,
          previous_runs: test.previous_runs,
          net_improvement: test.net_improvement,
        },
        jira_component: test.jira_component,
      },
    })

    // Cleanup: Clear context when component unmounts
    return () => {
      unsetPageContextForChat()
    }
  }, [
    isLoaded,
    test,
    testName,
    filterModel,
    props.release,
    setPageContextForChat,
    unsetPageContextForChat,
  ])

  const breadcrumbs = (
    <SimpleBreadcrumbs
      release={props.release}
      previousPage={<Link to={'/tests/' + props.release}>Tests</Link>}
      currentPage="Test Analysis"
    />
  )

  if (fetchError !== '') {
    return (
      <Fragment>
        {breadcrumbs}

        <Alert style={{ marginTop: 25 }} severity="error">
          {fetchError}
        </Alert>
      </Fragment>
    )
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <Fragment>
      <Container maxWidth="xl">
        <Typography variant="h3" style={{ textAlign: 'center' }}>
          Test Analysis
        </Typography>
        <Divider style={{ margin: 20 }} />
        <Grid container spacing={3} alignItems="stretch">
          <Grid item md={4}>
            <SummaryCard
              key="test-summary"
              threshold={TEST_THRESHOLDS}
              name="7 Day Overall"
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
              <TableContainer sx={{ marginBottom: 2 }}>
                <Table aria-label="simple table">
                  <TableBody>
                    <TableRow>
                      <TableCell colSpan={2}>
                        <Typography variant="h5">{testName}</Typography>
                      </TableCell>
                    </TableRow>
                    {test.jira_component ? (
                      <TableRow>
                        <TableCell scope="row">
                          <b>Jira component</b>
                        </TableCell>
                        <TableCell align="left">
                          {test.jira_component}
                        </TableCell>
                      </TableRow>
                    ) : (
                      <></>
                    )}
                  </TableBody>
                </Table>
              </TableContainer>

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
                Issues
                <Tooltip title="Issues links to all known Jira issues mentioning this test. Only OCPBUGS project is indexed, not the mirrored older bugs from Bugzilla. Issues are shown from all releases.">
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <BugTable testName={testName} />
              <Box
                sx={{
                  display: 'flex',
                  marginTop: 2,
                }}
              >
                <BugButton
                  jiraComponentID={test.jira_component_id}
                  testName={testName}
                />
              </Box>
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
                simpleLoading={true}
                pageSize={5}
                hideControls={true}
                collapse={false}
                release={props.release}
                sortField="delta_from_passing_average"
                sort="asc"
                filterModel={filterModel}
              />
            </Card>
          </Grid>

          <TestStackedChart
            release={props.release}
            test={testName}
            filter={filterModel}
          />

          <TestPassRateCharts
            test={testName}
            release={props.release}
            filterModel={filterModel}
            grouping="jobs"
          />

          <TestPassRateCharts
            test={testName}
            release={props.release}
            filterModel={filterModel}
            grouping="variants"
          />

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5}>
              <Typography variant="h5">
                Average Test Duration (Seconds)
                <Tooltip title={<p>Shows the average test duration by day.</p>}>
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <TestDurationChart
                release={props.release}
                test={testName}
                filterModel={filterModel}
              />
            </Card>
          </Grid>
          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5}>
              <Typography variant="h5">
                Most Recent Failure Outputs
                <Tooltip
                  title={
                    <p>
                      Shows the outputs from at most the last 10 failures with a
                      link to the Prow job run.
                    </p>
                  }
                >
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <Box
                style={{
                  maxHeight: '35vh',
                  minHeight: '35vh',
                  overflow: 'auto',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
              >
                <TestOutputs
                  release={props.release}
                  test={testName}
                  filterModel={filterModel}
                />
              </Box>
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
