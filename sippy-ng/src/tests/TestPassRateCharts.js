import { ArrayParam, useQueryParam } from 'use-query-params'
import {
  Button,
  Card,
  CircularProgress,
  Dialog,
  Grid,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { DataGrid } from '@material-ui/data-grid'
import { Line } from 'react-chartjs-2'
import { safeEncodeURIComponent } from '../helpers'
import { scale } from 'chroma-js'
import Alert from '@material-ui/lab/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import InfoIcon from '@material-ui/icons/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import { Link } from 'react-router-dom'

export default function TestPassRateCharts(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [groupedData, setGroupedData] = React.useState({})
  const [fetchError, setFetchError] = React.useState('')
  const [groupSelectionDialog, setGroupSelectionDialog] = React.useState(false)
  const [selectionModel, setSelectionModel] = React.useState([
    0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
  ])
  const [selectedGroups = [], setSelectedGroups] = useQueryParam(
    props.grouping,
    ArrayParam
  )
  const fetchData = () => {
    const filter = safeEncodeURIComponent(JSON.stringify(props.filterModel))

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/tests/analysis/${props.grouping}?release=${props.release}&test=${props.test}&filter=${filter}`
      ),
    ])
      .then(([apiResponse]) => {
        if (apiResponse.status !== 200) {
          throw new Error('server returned ' + apiResponse.status)
        }
        return Promise.all([apiResponse.json()])
      })
      .then(([apiResponse]) => {
        setGroupedData(apiResponse)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve test analysis for ' +
            props.release +
            +' ' +
            props.grouping +
            ' , ' +
            error
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
          <Card className="test-failure-card" elevation={5}>
            <Typography variant="h5">Pass Rate By {props.grouping}</Typography>
            <CircularProgress color="inherit" />
          </Card>
        </Grid>
      </Fragment>
    )
  }

  const colors = scale('Set2')
    .mode('lch')
    .colors(Object.keys(groupedData).length)

  // TODO: This is kind of awkward, but it's relatively fast. Need to create a list of all
  // dates we'll chart, and have it sorted.
  let daySet = new Set()
  Object.keys(groupedData).forEach((key) => {
    Object.keys(groupedData[key]).forEach((item) =>
      daySet.add(groupedData[key][item].date)
    )
  })
  let days = Array.from(daySet)
  days.sort((a, b) => new Date(a) - new Date(b))

  const chart = {
    labels: days,
    datasets: [],
  }

  const columns = [
    {
      field: 'id',
      hide: true,
      filterable: false,
    },
    {
      field: 'name',
      headerName: props.grouping,
      flex: 4,
    },
  ]

  const options = {
    parsing: {
      xAxisKey: 'date',
      yAxisKey: 'pass_percentage',
    },
    plugins: {
      tooltip: {
        callbacks: {
          label: function (context) {
            let data = groupedData[context.dataset.label].find(
              (element) => element.date === context.label
            )

            return `${context.dataset.label} ${data.pass_percentage}% (${data.passes} passed, ${data.failures} failed, ${data.flakes} flaked)`
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
  Object.keys(groupedData).forEach((group) => {
    chart.datasets.push({
      type: 'line',
      label: `${group}`,
      tension: 0.25,
      yAxisID: 'y',
      borderColor: group === 'overall' ? 'black' : colors[index],
      backgroundColor: group === 'overall' ? 'black' : colors[index],
      data: groupedData[group],
      order: group === 'overall' ? 0 : 1,
    })
    index++
  })

  return (
    <Fragment>
      <Grid item md={12}>
        <Card className="test-failure-card" elevation={5}>
          <Typography variant="h5">
            Pass Rate By{' '}
            {props.grouping.charAt(0).toUpperCase() +
              props.grouping.substr(1).toLowerCase()}
          </Typography>
          <Line data={chart} options={options} height={80} />
          <Button
            style={{ marginTop: 20 }}
            variant="contained"
            color="secondary"
            onClick={() => setGroupSelectionDialog(true)}
          >
            Select {props.grouping} to chart
          </Button>
          <Dialog
            fullWidth={true}
            maxWidth="lg"
            onClose={() => setGroupSelectionDialog(false)}
            open={groupSelectionDialog}
          >
            <Grid className="test-dialog">
              <Typography
                variant="h6"
                style={{ marginTop: 20, marginBottom: 20 }}
              >
                Select {props.grouping} to chart
              </Typography>
              <DataGrid
                components={{ Toolbar: GridToolbar }}
                columns={columns}
                rows={allTests}
                pageSize={10}
                rowHeight={60}
                autoHeight={true}
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
                onClick={() => setGroupSelectionDialog(false)}
              >
                OK
              </Button>
            </Grid>
          </Dialog>
        </Card>
      </Grid>
    </Fragment>
  )
}

TestPassRateCharts.defaultProps = {
  grouping: 'variants',
}

TestPassRateCharts.propTypes = {
  grouping: PropTypes.string,
  release: PropTypes.string.isRequired,
  test: PropTypes.string.isRequired,
  filterModel: PropTypes.object.isRequired,
}
