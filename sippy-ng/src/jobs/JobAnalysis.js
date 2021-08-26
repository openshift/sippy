import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import { ArrayParam, JsonParam, useQueryParam } from 'use-query-params'
import Alert from '@material-ui/lab/Alert'
import { Box, Button, Card, Container, Dialog, Grid, Typography } from '@material-ui/core'
import './JobAnalysis.css'
import Divider from '@material-ui/core/Divider'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import { Link } from 'react-router-dom'
import { DataGrid } from '@material-ui/data-grid'
import { JobStackedChart } from './JobStackedChart'
import { Line } from 'react-chartjs-2'
import { scale } from 'chroma-js'
import GridToolbar from '../datagrid/GridToolbar'
import { explainFilter, pathForJobRunsWithFilter, pathForJobsWithFilter, withSort } from '../helpers'
import SummaryCard from '../components/SummaryCard'
import { JOB_THRESHOLDS } from '../constants'

export function JobAnalysis (props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [analysis, setAnalysis] = React.useState({ by_day: {} })
  const [filterModel] = useQueryParam('filters', JsonParam)

  const [fetchError, setFetchError] = React.useState('')

  const [allTests, setAllTests] = React.useState([])
  const [selectionModel, setSelectionModel] = React.useState([0, 1, 2, 3, 4])
  const [selectedTests = [], setSelectedTests] = useQueryParam('tests', ArrayParam)
  const [testFilter = { items: [] }, setTestFilter] = useQueryParam('testFilters', JsonParam)
  const [testSelectionDialog, setTestSelectionDialog] = React.useState(false)

  const fetchData = () => {
    document.title = `Sippy > ${props.release} > Jobs > Analysis`

    let queryParams = `release=${props.release}`
    if (filterModel) {
      queryParams += `&filter=${encodeURIComponent(JSON.stringify(filterModel))}`
    }

    Promise.all([
      fetch(`${process.env.REACT_APP_API_URL}/api/jobs/analysis?${queryParams}`)
    ])
      .then(([analysis]) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        return Promise.all([analysis.json()])
      })
      .then(([analysis]) => {
        setAnalysis(analysis)

        let allTests = new Map()
        Object.keys(analysis.by_day)
          .map((key) => Object.keys(analysis.by_day[key].test_count)
            .forEach((test) => {
              const count = allTests.get(test) || { name: test, value: 0 }
              allTests.set(test, { name: test, value: count.value + analysis.by_day[key].test_count[test] })
            }))

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
      }
      ).catch(error => {
        setFetchError('Could not retrieve job analysis ' + props.release + ', ' + error)
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

  const topTestChart = {
    labels: Object.keys(analysis.by_day),
    datasets: []
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
        data: Object.keys(analysis.by_day).map((key) => (100 * (1 - (analysis.by_day[key].test_count[allTests[id].name] || 0) / analysis.by_day[key].total_runs)))
      })
    }
  })

  const options = {
    plugins: {
      tooltip: {
        callbacks: {
          label: function (context) {
            const failures = analysis.by_day[context.label].test_count[context.dataset.label] || 0
            const runs = analysis.by_day[context.label].total_runs

            return `${context.dataset.label} ${context.raw.toFixed(2)}% (${failures} failed of ${runs} runs)`
          }
        }
      }
    },
    scales: {
      y: {
        max: 100,
        ticks: {
          callback: (value, index, values) => {
            return `${value}%`
          }
        }
      }
    }
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
      value: searchValue
    })
    setTestFilter(currentFilters)
  }

  const updateSelectionModel = (m) => {
    setSelectedTests(allTests.filter((row) => m.includes(row.id)).map((row) => row.name))
    setSelectionModel(m)
  }

  const totalSuccess = Object.keys(analysis.by_day)
    .map((key) => analysis.by_day[key].result_count.S || 0)
    .reduce((acc, val) => acc + val)

  const totalRuns = Object.keys(analysis.by_day)
    .map((key) => analysis.by_day[key].total_runs)
    .reduce((acc, val) => acc + val)

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={
          <Link
            to={pathForJobsWithFilter(props.release, filterModel)}>
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
                <Fragment>{totalRuns} runs, {totalSuccess} successful</Fragment>
              }
            />
          </Grid>

          <Grid item md={8}>
            <Card className="test-failure-card" elevation={5} style={{ height: '100%' }}>
              <strong>Current filter</strong>:<br />
              Showing jobs matching {explainFilter(filterModel)}<br />
              <Divider />
              <Button
                variant="contained"
                color="primary"
                component={Link}
                style={{ marginTop: 20, marginRight: 20 }}
                to={pathForJobsWithFilter(props.release, filterModel)}
              >
                View matching jobs
              </Button>

              <Button
                variant="contained"
                color="secondary"
                component={Link}
                style={{ marginTop: 20 }}
                to={withSort(pathForJobRunsWithFilter(props.release, filterModel), 'timestamp', 'desc')}
              >
                View matching job runs
              </Button>
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="job-failure-card" elevation={5}>
              <Typography variant="h5">
                Job results
              </Typography>
              <JobStackedChart release={props.release} analysis={analysis} />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card className="job-failure-card" elevation={5}>
              <Typography variant="h5">
                Test results for selected jobs
              </Typography>
              <Box hidden={allTests.length !== 0}>
                <p>No test data found.</p>
              </Box>
              <Box hidden={allTests.length === 0}>
                <Box hidden={selectionModel.length === 0}>
                  <Line data={topTestChart} options={options} height={100} />
                </Box>
                <Button style={{ marginTop: 20 }} variant="contained" color="secondary" onClick={() => setTestSelectionDialog(true)}>Select tests to chart</Button>
                <Dialog fullWidth={true} maxWidth="lg" onClose={() => setTestSelectionDialog(false)} open={testSelectionDialog}>
                  <Grid className="test-dialog">
                    <Typography variant="h6" style={{ marginTop: 20, marginBottom: 20 }}>
                      Select tests to chart
                    </Typography>
                    <DataGrid
                      components={{ Toolbar: GridToolbar }}
                      columns={
                        [
                          {
                            field: 'id',
                            hide: true,
                            filterable: false
                          },
                          {
                            field: 'name',
                            headerName: 'Test name',
                            flex: 4,
                            renderCell: (param) => (
                              <div className="job-name">
                                {param.value}
                              </div>
                            )
                          },
                          {
                            field: 'value',
                            type: 'number',
                            headerName: 'Failure count',
                            flex: 1
                          }
                        ]
                      }
                      rows={allTests}
                      pageSize={10}
                      rowHeight={60}
                      autoHeight={true}
                      selectionModel={selectionModel}
                      onSelectionModelChange={(m) => updateSelectionModel(m)}
                      filterModel={testFilter}
                      onFilterModelChange={(m) => { setTestFilter(m) }}
                      checkboxSelection
                      componentsProps={{
                        toolbar: {
                          setFilterModel: setTestFilter,
                          clearSearch: () => requestSearch(''),
                          doSearch: requestSearch
                        }
                      }}
                    />

                    <Button style={{ marginTop: 20 }} variant="contained" color="primary" onClick={() => setTestSelectionDialog(false)}>OK</Button>
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
  job: ''
}

JobAnalysis.propTypes = {
  release: PropTypes.string.isRequired,
  job: PropTypes.string
}
