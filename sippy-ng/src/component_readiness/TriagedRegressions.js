import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { jiraUrlPrefix } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { Typography } from '@mui/material'
import InfoIcon from '@mui/icons-material/Info'
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

    if (
      selectedTriagedEntry.regressions !== 'undefined' &&
      selectedTriagedEntry.regressions.length > 0
    ) {
      props.eventEmitter.emit(
        'triagedEntrySelectionChanged',
        selectedTriagedEntry.regressions
      )
    }
  }

  const columns = [
    {
      field: 'description',
      valueGetter: (value) => {
        return value.row.description
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
        const url = value.row.url
        const val = {
          url,
          text: url,
        }
        if (url.startsWith(jiraUrlPrefix)) {
          val.text = url.slice(jiraUrlPrefix.length)
        }
        return val
      },
      headerName: 'Jira',
      flex: 5,
      renderCell: (param) => (
        <a target="_blank" href={param.value.url} rel="noreferrer">
          <div className="test-name">{param.value.text}</div>
        </a>
      ),
    },
    {
      field: 'resolution_date',
      valueGetter: (value) => {
        return value.row.resolved?.Valid ? value.row.resolved.Time : ''
      },
      headerName: 'Resolution Date',
      flex: 5,
      renderCell: (param) => <div>{param.value}</div>,
    },
    {
      field: 'bug_state',
      valueGetter: (value) => {
        const bugState = value.row.bug?.status || ''
        return bugState
      },
      headerName: 'State',
      flex: 5,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'bug_version',
      valueGetter: (value) => {
        const bugState =
          value.row.bug?.target_versions ||
          value.row.bug?.affects_versions ||
          ''
        return bugState
      },
      headerName: 'Version',
      flex: 5,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'details',
      valueGetter: (value) => {
        return value.row.id
      },
      headerName: 'Details',
      flex: 2,
      renderCell: (param) => (
        <Link to={'/triages/' + param.value}>
          <InfoIcon />
        </Link>
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
