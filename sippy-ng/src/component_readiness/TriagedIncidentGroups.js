import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function TriagedIncidentGroups(props) {
  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

  const handleSetSelectionModel = (event) => {
    let selectedIncident = {}
    props.triagedIncidents.forEach((incident) => {
      if (event[0] === incident.group_id) selectedIncident = incident
    })

    props.eventEmitter.emit(
      'triagedRegressionGroupSelectionChanged',
      selectedIncident.grouped_incidents.incidents
    )
  }

  // define table columns
  const columns = [
    {
      field: 'description',
      valueGetter: (value) => {
        return value.row.grouped_incidents.issue.description
      },
      headerName: 'Description',
      flex: 20,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'url',
      valueGetter: (value) => {
        return value.row.grouped_incidents.issue.url
      },
      headerName: 'Jira',
      flex: 12,
      renderCell: (param) => (
        <a target="_blank" href={param.value} rel="noreferrer">
          <div className="test-name">{param.value}</div>
        </a>
      ),
    },
  ]

  return (
    <Fragment>
      <Typography>Incident Groups</Typography>
      <DataGrid
        sortModel={sortModel}
        onSortModelChange={setSortModel}
        onSelectionModelChange={handleSetSelectionModel}
        components={{ Toolbar: GridToolbar }}
        rows={props.triagedIncidents}
        columns={columns}
        getRowId={(row) => row.group_id}
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
    </Fragment>
  )
}

TriagedIncidentGroups.propTypes = {
  eventEmitter: PropTypes.object,
  triagedIncidents: PropTypes.array,
}
