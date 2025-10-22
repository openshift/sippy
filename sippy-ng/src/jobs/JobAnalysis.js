import './JobAnalysis.css'
import {
  ArrayParam,
  NumberParam,
  StringParam,
  useQueryParam,
  withDefault,
} from 'use-query-params'
import { ArrowBack, ArrowForward } from '@mui/icons-material'
import {
  Box,
  Button,
  Card,
  Container,
  Dialog,
  Grid,
  Tooltip,
  Typography,
} from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { filterList } from '../datagrid/utils'
import { getColumns, getViews } from './JobTable'
import {
  getReportStartDate,
  pathForJobRunsWithFilter,
  pathForJobsWithFilter,
  safeEncodeURIComponent,
  SafeJSONParam,
  withSort,
} from '../helpers'
import { GridView } from '../datagrid/GridView'
import { hourFilter, JobStackedChart } from './JobStackedChart'
import { JOB_THRESHOLDS } from '../constants'
import { Line } from 'react-chartjs-2'
import { Link } from 'react-router-dom'
import { ReportEndContext } from '../App'
import { scale } from 'chroma-js'
import { usePageContextForChat } from '../chat/store/useChatStore'
import Alert from '@mui/material/Alert'
import BugTable from '../bugs/BugTable'
import Divider from '@mui/material/Divider'
import GridToolbar from '../datagrid/GridToolbar'
import GridToolbarFilterMenu from '../datagrid/GridToolbarFilterMenu'
import InfoIcon from '@mui/icons-material/Info'
import JobTable from './JobTable'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import SummaryCard from '../components/SummaryCard'

export function JobAnalysis(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [analysis, setAnalysis] = React.useState({ by_period: {} })
  const [bugsURL, setBugsURL] = React.useState('')
  const { setPageContextForChat, unsetPageContextForChat } =
    usePageContextForChat()

  const [filterModel, setFilterModel] = useQueryParam('filters', SafeJSONParam)
  const [period, setPeriod] = useQueryParam('period', StringParam)
  const [dayOffset = 1, setDayOffset] = useQueryParam('dayOffset', NumberParam)

  const [fetchError, setFetchError] = React.useState('')

  const [allTests, setAllTests] = React.useState([])
  const [selectionModel, setSelectionModel] = React.useState([0, 1, 2, 3, 4])
  const [selectedTests = [], setSelectedTests] = useQueryParam(
    'tests',
    ArrayParam
  )
  const [testFilter, setTestFilter] = useQueryParam(
    'testFilters',
    withDefault(SafeJSONParam, { items: [] })
  )

  const [testSelectionDialog, setTestSelectionDialog] = React.useState(false)
  const startDate = getReportStartDate(React.useContext(ReportEndContext))

  const fetchData = () => {
    document.title = `Sippy > ${props.release} > Jobs > Analysis`

    let queryParams = `release=${props.release}`
    if (filterModel) {
      queryParams += `&filter=${safeEncodeURIComponent(
        JSON.stringify(filterModel)
      )}`
    }

    if (period) {
      queryParams += `&period=${period}`
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
        setBugsURL(
          `${process.env.REACT_APP_API_URL}/api/jobs/bugs?${queryParams}`
        )

        // allTests maps each test name to a struct containing the test name again, and the total number of
        // failures in the past 7 days. This value is used to sort on and determine the most relevant tests
        // to preselect in the graph.

        let allTests = new Map()
        let twoWeeksAgo = new Date(+startDate - 1000 * 60 * 60 * 24 * 7)
        Object.keys(analysis.by_period).map((key) =>
          Object.keys(analysis.by_period[key].test_count).forEach((test) => {
            const count = allTests.get(test) || { name: test, value: 0 }

            // We are most interested in the test failures for the last 7 days. We'll graph the whole timeframe we
            // have, but here we skip counts of failures before 7 days ago.
            let keyDate = new Date(Date.parse(key))
            let countChange = 0
            if (keyDate >= twoWeeksAgo) {
              countChange = analysis.by_period[key].test_count[test]
            }

            allTests.set(test, {
              name: test,
              value: count.value + countChange,
            })
          })
        )

        allTests = [...allTests.values()].sort((a, b) => b.value - a.value)

        const selected = []
        // DataGrid tables need an ID:
        for (let i = 0; i < allTests.length; i++) {
          if (selectedTests && selectedTests.includes(allTests[i].name)) {
            selected.push(i)
          }
          allTests[i].id = i
        }

        if (selected.length > 0) {
          setSelectionModel(selected)
        }

        setAllTests(allTests)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve job analysis ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [filterModel, period])

  // Update page context for chat
  useEffect(() => {
    if (!isLoaded || !analysis) return

    setPageContextForChat({
      page: 'job-analysis',
      url: window.location.href,
      instructions: `The user is viewing job analysis for multiple jobs matching specific filters.
        You can use your database query tools to answer additional questions about the jobs being viewed.
        When querying the database, apply the same filters shown in the context, especially the variant filters.`,
      suggestedQuestions: [
        'What are the most common test failures across these jobs?',
        'Which jobs have the lowest pass rates?',
      ],
      data: {
        release: props.release,
        filters: filterModel,
      },
    })

    // Cleanup: Clear context when component unmounts
    return () => {
      unsetPageContextForChat()
    }
  }, [
    isLoaded,
    analysis,
    filterModel,
    props.release,
    setPageContextForChat,
    unsetPageContextForChat,
  ])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  const topTestChart = {
    labels: Object.keys(analysis.by_period),
    datasets: [],
  }

  const colors = scale('Set2').mode('lch').colors(selectionModel.length)

  selectionModel.forEach((id, index) => {
    if (id < allTests.length) {
      topTestChart.datasets.push({
        type: 'line',
        label: allTests[id].name,
        backgroundColor: colors[index],
        borderColor: colors[index],
        tension: 0.3,
        data: Object.keys(analysis.by_period).map(
          (key) =>
            100 *
            (1 -
              (analysis.by_period[key].test_count[allTests[id].name] || 0) /
                analysis.by_period[key].total_runs)
        ),
      })
    }
  })

  const options = {
    plugins: {
      tooltip: {
        callbacks: {
          label: function (context) {
            const failures =
              analysis.by_period[context.label].test_count[
                context.dataset.label
              ] || 0
            const runs = analysis.by_period[context.label].total_runs

            return `${context.dataset.label} ${context.raw.toFixed(
              2
            )}% (${failures} failed of ${runs} runs)`
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

  const requestSearch = (searchValue) => {
    const currentFilters = testFilter
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'name'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'name',
      operatorValue: 'contains',
      value: searchValue,
    })
    setTestFilter(currentFilters)
  }

  const updateSelectionModel = (m) => {
    setSelectedTests(
      allTests.filter((row) => m.includes(row.id)).map((row) => row.name)
    )
    setSelectionModel(m)
  }

  const totalSuccess = Object.keys(analysis.by_period)
    .map((key) => analysis.by_period[key].result_count.S || 0)
    .reduce((acc, val) => acc + val, 0)

  const totalRuns = Object.keys(analysis.by_period)
    .map((key) => analysis.by_period[key].total_runs)
    .reduce((acc, val) => acc + val, 0)

  const columns = [
    {
      field: 'id',
      hide: true,
      filterable: false,
    },
    {
      field: 'name',
      headerName: 'Test name',
      flex: 4,
      renderCell: (param) => (
        <div className="job-name">
          <Link
            to={`/tests/${props.release}/analysis?test=${safeEncodeURIComponent(
              param.value
            )}`}
          >
            {param.value}
          </Link>
        </div>
      ),
    },
    {
      field: 'value',
      type: 'number',
      headerName: 'Failure count (7 day)',
      flex: 1,
    },
  ]

  const updateOffset = (newOffset) => {
    const newFilters = []
    filterModel &&
      filterModel.items.forEach((filter) => {
        if (filter.columnField !== 'timestamp') {
          newFilters.push(filter)
        }
      })
    newFilters.push(...hourFilter(newOffset, startDate))
    setFilterModel({
      items: newFilters,
      linkOperator: filterModel ? filterModel.linkOperator : 'and',
    })
    setDayOffset(newOffset)
  }

  const togglePeriod = () => {
    const newPeriod = period === 'hour' ? 'day' : 'hour'

    const newFilters = []
    filterModel &&
      filterModel.items.forEach((filter) => {
        if (filter.columnField !== 'timestamp') {
          newFilters.push(filter)
        }
      })

    if (newPeriod === 'hour') {
      newFilters.push(...hourFilter(dayOffset, startDate))
    }

    setFilterModel({
      items: newFilters,
      linkOperator: filterModel ? filterModel.linkOperator : 'and',
    })
    setPeriod(newPeriod)
  }

  function withoutJobRunFilters(filterModel) {
    let jobRunFilters = ['cluster', 'timestamp']

    if (!filterModel || filterModel.items === []) {
      return filterModel
    }

    let newFilters = []
    filterModel.items.forEach((filter) => {
      if (!jobRunFilters.includes(filter.columnField)) {
        newFilters.push(filter)
      }
    })

    return {
      items: newFilters,
      not: filterModel.not,
      linkOperator: filterModel.linkOperator,
    }
  }

  const gridView = new GridView(getColumns(props), getViews(props), 'Default')

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={
          <Link
            to={pathForJobsWithFilter(
              props.release,
              withoutJobRunFilters(filterModel)
            )}
          >
            Jobs
          </Link>
        }
        currentPage="Job Analysis"
      />

      <Container size="xl">
        <Typography variant="h3" style={{ textAlign: 'center' }}>
          Job Analysis
        </Typography>
        <Divider style={{ margin: 20 }} />
        <Grid container spacing={3}>
          <Grid item md={4}>
            <SummaryCard
              key="test-summary"
              threshold={JOB_THRESHOLDS}
              name="Overall"
              success={totalSuccess}
              fail={totalRuns - totalSuccess}
              flakes={0}
              caption={
                <Fragment>
                  {totalRuns} runs, {totalSuccess} successful
                </Fragment>
              }
            />
          </Grid>

          <Grid item md={8}>
            <Card
              className="test-failure-card"
              elevation={5}
              style={{ height: '100%' }}
            >
              <strong>Current filter</strong>
              <br />
              {filterList(filterModel, setFilterModel)}
              <br />
              <Divider style={{ marginBottom: 20 }} />
              <Grid container justifyContent="space-between">
                <GridToolbarFilterMenu
                  standalone={true}
                  filterModel={filterModel || { items: [] }}
                  setFilterModel={setFilterModel}
                  columns={[
                    {
                      field: 'timestamp',
                      headerName: 'Date / Time',
                      filterable: true,
                      type: 'date',
                    },
                    {
                      field: 'cluster',
                      headerName: 'Build cluster',
                      autocomplete: 'cluster',
                      filterable: true,
                      type: 'string',
                    },
                    ...gridView.filterColumns,
                  ]}
                />
                <Button variant="contained" onClick={togglePeriod}>
                  View by {period === 'hour' ? 'day' : 'hour'}
                </Button>

                <Button
                  variant="contained"
                  color="primary"
                  component={Link}
                  style={{ marginLeft: 20, marginRight: 20 }}
                  to={pathForJobsWithFilter(
                    props.release,
                    withoutJobRunFilters(filterModel)
                  )}
                >
                  View matching jobs
                </Button>
                <Button
                  variant="contained"
                  color="secondary"
                  component={Link}
                  to={withSort(
                    pathForJobRunsWithFilter(props.release, filterModel),
                    'timestamp',
                    'desc'
                  )}
                >
                  View matching job runs
                </Button>
              </Grid>
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="job-variants-card" elevation={5}>
              <Typography variant="h5">
                Matching Jobs
                <Tooltip title="Matching Jobs shows all jobs matching the selected filters">
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <JobTable
                view="Variants"
                hideControls={true}
                pageSize={5}
                release={props.release}
                filterModel={filterModel}
              />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="test-failure-card" elevation={5}>
              <Typography variant="h5">
                Issues
                <Tooltip title="Issues links to all known Jira issues mentioning this job. Only OCPBUGS project is indexed, not the mirrored older bugs from Bugzilla. Issues are shown from all releases.">
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <BugTable bugsURL={bugsURL} />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="job-failure-card" elevation={5}>
              <Typography variant="h5">Job results</Typography>
              <JobStackedChart
                release={props.release}
                analysis={analysis}
                filter={filterModel}
              />
              {period === 'hour' ? (
                <div align="center">
                  <Button
                    onClick={() => updateOffset(dayOffset + 1)}
                    startIcon={<ArrowBack />}
                  />
                  <Button
                    style={dayOffset > 1 ? {} : { display: 'none' }}
                    onClick={() => dayOffset > 1 && updateOffset(dayOffset - 1)}
                    startIcon={<ArrowForward />}
                  />
                </div>
              ) : (
                ''
              )}
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="job-failure-card" elevation={5}>
              <Typography variant="h5">
                Test results for selected jobs
                <Tooltip
                  title={
                    'By default, this plots the top 5 tests with the most failures in ' +
                    'the set of jobs selected over the past 7 days. Click the button below to change the list of ' +
                    'tests to plot. Only tests with at least one failure during the reporting period are available.'
                  }
                >
                  <InfoIcon />
                </Tooltip>
              </Typography>
              <Box hidden={allTests.length !== 0}>
                <p>No test data found.</p>
              </Box>
              <Box hidden={allTests.length === 0}>
                <Box hidden={selectionModel.length === 0}>
                  <Line data={topTestChart} options={options} height={100} />
                </Box>
                <Button
                  style={{ marginTop: 20 }}
                  variant="contained"
                  color="secondary"
                  onClick={() => setTestSelectionDialog(true)}
                >
                  Select/view tests to chart
                </Button>
                <Dialog
                  fullWidth={true}
                  maxWidth="lg"
                  onClose={() => setTestSelectionDialog(false)}
                  open={testSelectionDialog}
                >
                  <Grid className="test-dialog">
                    <Typography
                      variant="h6"
                      style={{ marginTop: 20, marginBottom: 20 }}
                    >
                      Select tests to chart
                    </Typography>
                    <DataGrid
                      components={{ Toolbar: GridToolbar }}
                      columns={columns}
                      rows={allTests}
                      pageSize={10}
                      rowHeight={60}
                      autoHeight={true}
                      filterModel={testFilter}
                      selectionModel={selectionModel}
                      onSelectionModelChange={(m) => updateSelectionModel(m)}
                      checkboxSelection
                      componentsProps={{
                        toolbar: {
                          columns: columns,
                          filterModel: testFilter,
                          setFilterModel: setTestFilter,
                          clearSearch: () => requestSearch(''),
                          doSearch: requestSearch,
                        },
                      }}
                    />

                    <Button
                      style={{ marginTop: 20 }}
                      variant="contained"
                      color="primary"
                      onClick={() => setTestSelectionDialog(false)}
                    >
                      OK
                    </Button>
                  </Grid>
                </Dialog>
              </Box>
            </Card>
          </Grid>
        </Grid>
      </Container>
    </Fragment>
  )
}

JobAnalysis.defaultProps = {
  job: '',
}

JobAnalysis.propTypes = {
  release: PropTypes.string.isRequired,
  job: PropTypes.string,
}
