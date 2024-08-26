import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { Typography } from '@mui/material'
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
      field: 'test_name',
      valueGetter: (value) => {
        return value.row.test_name
      },
      headerName: 'Test',
      flex: 20,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'architecture',
      valueGetter: (value) => {
        let showValue = 'Missing'
        value.row.variants.forEach((variant) => {
          if ('key' in variant && variant['key'] === 'Architecture') {
            if ('value' in variant) {
              showValue = variant['value']
            }
          }
        })
        return showValue
      },
      headerName: 'Architecture',
      flex: 20,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'network',
      valueGetter: (value) => {
        let showValue = 'Missing'
        value.row.variants.forEach((variant) => {
          if ('key' in variant && variant['key'] === 'Network') {
            if ('value' in variant) {
              showValue = variant['value']
            }
          }
        })
        return showValue
      },
      headerName: 'Network',
      flex: 12,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'platform',
      valueGetter: (value) => {
        let showValue = 'Missing'
        value.row.variants.forEach((variant) => {
          if ('key' in variant && variant['key'] === 'Platform') {
            if ('value' in variant) {
              showValue = variant['value']
            }
          }
        })
        return showValue
      },
      headerName: 'Platform',
      flex: 12,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'upgrade',
      valueGetter: (value) => {
        let showValue = 'Missing'
        value.row.variants.forEach((variant) => {
          if ('key' in variant && variant['key'] === 'Upgrade') {
            if ('value' in variant) {
              showValue = variant['value']
            }
          }
        })
        return showValue
      },
      headerName: 'Upgrade',
      flex: 12,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'variant', // gets replaced with pantfusi
      valueGetter: (value) => {
        let showValue = 'Missing'
        value.row.variants.forEach((variant) => {
          if ('key' in variant && variant['key'] === 'Variant') {
            if ('value' in variant) {
              showValue = variant['value']
            }
          }
        })
        return showValue
      },
      headerName: 'Variant',
      flex: 12,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
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
