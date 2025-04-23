import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function TriagedRegressions(props) {
  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

  const handleSetSelectionModel = (event) => {
    let selectedTriagedEntry = {}
    props.triageEntries.forEach((entry) => {
      if (event[0] === entry.id) selectedTriagedEntry = entry
    })

    props.eventEmitter.emit(
      'triagedEntrySelectionChanged',
      selectedTriagedEntry.regressions
    )
  }

  const columns = [
    {
      field: 'description',
      valueGetter: (value) => {
        return 'Description is not yet tracked'
      },
      headerName: 'Description',
      flex: 20,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'type',
      valueGetter: (value) => {
        return value.row.type
      },
      headerName: 'Type',
      flex: 5,
      renderCell: (param) => <div>{param.value}</div>,
    },
    {
      field: 'url',
      valueGetter: (value) => {
        return value.row.url
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
      <Typography>Triaged Test Regressions</Typography>
      <DataGrid
        sortModel={sortModel}
        onSortModelChange={setSortModel}
        onSelectionModelChange={handleSetSelectionModel}
        components={{ Toolbar: GridToolbar }}
        rows={props.triageEntries}
        columns={columns}
        getRowId={(row) => row.id}
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

TriagedRegressions.propTypes = {
  eventEmitter: PropTypes.object,
  triageEntries: PropTypes.array,
}
