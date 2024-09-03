import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { formColumnName } from './CompReadyUtils'
import { relativeTime } from '../helpers'
import { Tooltip, Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function TriagedVariants(props) {
  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

  const [groupedIncidents, setGroupedIncidents] = React.useState([])
  const [showView, setShowView] = React.useState(false)

  const handleTriagedRegressionGroupSelectionChanged = (data) => {
    let displayView = false
    if (data) {
      displayView = true
    }
    setGroupedIncidents(data)
    setShowView(displayView)
  }
  props.eventEmitter.on(
    'triagedRegressionGroupSelectionChanged',
    handleTriagedRegressionGroupSelectionChanged
  )

  const handleSetSelectionModel = (event) => {
    let selectedIncident = {}
    groupedIncidents.forEach((incident) => {
      if (event[0] === incident.incident_id) selectedIncident = incident
    })
    props.eventEmitter.emit(
      'triagedRegressionVariantSelectionChanged',
      selectedIncident
    )
  }

  // define table columns
  const columns = [
    {
      field: 'component',
      headerName: 'Component',
      flex: 20,
      valueGetter: (params) => {
        return params.row.details.component
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'capability',
      headerName: 'Capability',
      flex: 12,
      valueGetter: (params) => {
        return params.row.details.capability
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'test_name',
      headerName: 'Test Name',
      flex: 40,
      valueGetter: (params) => {
        return params.row.details.test_name
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'test_suite',
      headerName: 'Test Suite',
      flex: 15,
      valueGetter: (params) => {
        return params.row.details.test_suite
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'variants',
      headerName: 'Variants',
      flex: 30,
      valueGetter: (params) => {
        return formColumnName({ variants: params.row.details.variants })
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'opened',
      headerName: 'Regressed Since',
      flex: 12,
      valueGetter: (params) => {
        if (!params.row.details.opened) {
          // For a regression we haven't yet detected:
          return ''
        }
        const regressedSinceDate = new Date(params.row.details.opened)
        return relativeTime(regressedSinceDate, new Date())
      },
      renderCell: (param) => (
        <Tooltip title="WARNING: This is the first time we detected this test regressed in the default query. This value is not relevant if you've altered query parameters from the default.">
          <div className="regressed-since">{param.value}</div>
        </Tooltip>
      ),
    },
  ]

  return (
    <Fragment>
      <div hidden={!showView} className="cr-triage-panel-element">
        <Typography>Test Failures</Typography>
        <DataGrid
          sortModel={sortModel}
          onSortModelChange={setSortModel}
          onSelectionModelChange={handleSetSelectionModel}
          components={{ Toolbar: GridToolbar }}
          rows={groupedIncidents}
          columns={columns}
          getRowId={(row) => row.incident_id}
          pageSize={10}
          rowHeight={60}
          autoHeight={true}
          checkboxSelection={false}
          componentsProps={{
            toolbar: {
              columns: columns,
            },
          }}
        />
      </div>
    </Fragment>
  )
}

TriagedVariants.propTypes = {
  eventEmitter: PropTypes.object,
}
