import { CheckCircle, Error as ErrorIcon } from '@mui/icons-material'
import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { formatDateToSeconds, relativeTime } from '../helpers'
import { jiraUrlPrefix } from './CompReadyUtils'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { Tooltip, Typography } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import CompSeverityIcon from './CompSeverityIcon'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

export default function TriagedRegressions({
  triageEntries,
  eventEmitter,
  entriesPerPage = 10,
  allRegressedTests,
}) {
  const theme = useTheme()
  const [sortModel, setSortModel] = React.useState([
    { field: 'created_at', sort: 'desc' },
  ])

  const [activeRow, setActiveRow] = useQueryParam(
    'regressedModalRow',
    StringParam, //String is used in order to re-use the same parameter that is utilized in the rest of the modal's tabs
    { updateType: 'replaceIn' }
  )
  const [activePage, setActivePage] = useQueryParam(
    'regressedModalPage',
    NumberParam,
    { updateType: 'replaceIn' }
  )
  const [activeRegression] = useQueryParam(
    'regressedModalTestRow',
    NumberParam,
    { updateType: 'replaceIn' }
  )

  useEffect(() => {
    if (activeRow) {
      const triage = getSelectedTriage(activeRow)
      if (triage) {
        toggleAssociatedRegressions(triage)
      }
    }
  }, [])

  const getSelectedTriage = (id) => {
    return triageEntries.find((triage) => String(triage.id) === id)
  }

  function toggleAssociatedRegressions(triage) {
    if (triage.regressions !== null && triage.regressions.length > 0) {
      // When the new selection doesn't contain the active regression id, we should allow it to be cleared
      const triageContainsCurrentlySelectedRegression = triage.regressions.find(
        (r) => r.id === activeRegression
      )
      const data = {
        regressions: triage.regressions,
        activeId: triageContainsCurrentlySelectedRegression
          ? activeRegression
          : undefined,
      }
      eventEmitter.emit('triagedEntrySelectionChanged', data)
    }
  }

  const handleSetSelectionModel = (event) => {
    const triage = getSelectedTriage(event[0])
    if (triage) {
      setActiveRow(String(triage.id), 'replaceIn')
      toggleAssociatedRegressions(triage)
    }
  }

  // Helper function to check if triage has any regressions with status -1000
  const hasStatus1000Regression = (triage) => {
    if (
      !allRegressedTests ||
      !allRegressedTests.length ||
      !triage.regressions
    ) {
      return false
    }

    // Get regression IDs from this triage
    const triageRegressionIds = triage.regressions.map((r) => r.id)

    // Filter allRegressedTests to find those matching this triage's regressions
    const relevantRegressedTests = allRegressedTests.filter(
      (rt) =>
        rt?.regression?.id && triageRegressionIds.includes(rt.regression.id)
    )

    // Check if any have status -1000
    return relevantRegressedTests.some((rt) => rt.status === -1000)
  }

  const columns = [
    {
      field: 'resolution_date',
      valueGetter: (value) => {
        return value.row.resolved?.Valid ? value.row.resolved.Time : ''
      },
      headerName: 'Resolved',
      flex: 4,
      align: 'center',
      renderCell: (param) => {
        const triage = triageEntries.find((t) => t.id === param.row.id)
        const hasStatus1000 = hasStatus1000Regression(triage)

        return param.value ? (
          <Tooltip
            title={`${relativeTime(
              new Date(param.value),
              new Date()
            )} (${formatDateToSeconds(param.value)})`}
          >
            {hasStatus1000 ? (
              <CompSeverityIcon status={-1000} />
            ) : (
              <CheckCircle style={{ color: theme.palette.success.light }} />
            )}
          </Tooltip>
        ) : (
          <Tooltip title="Not resolved">
            <ErrorIcon style={{ color: theme.palette.error.light }} />
          </Tooltip>
        )
      },
    },
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
      field: 'bug_state',
      valueGetter: (value) => {
        return value.row.bug?.status || ''
      },
      headerName: 'State',
      flex: 5,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'bug_version',
      valueGetter: (value) => {
        return (
          value.row.bug?.target_versions ||
          value.row.bug?.affects_versions ||
          ''
        )
      },
      headerName: 'Version',
      flex: 5,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'last_change',
      valueGetter: (value) => {
        return value.row.bug?.last_change_time || ''
      },
      headerName: 'Jira updated',
      flex: 5,
      renderCell: (param) => (
        <Tooltip title={param.value}>
          <div className="test-name">
            {relativeTime(new Date(param.value), new Date())}
          </div>
        </Tooltip>
      ),
    },
    {
      field: 'created_at',
      hide: true,
      valueGetter: (value) => {
        return value.row.created_at
      },
      headerName: 'Created at',
      flex: 5,
      renderCell: (param) => (
        <Tooltip title={param.value}>
          <div className="test-name">
            {relativeTime(new Date(param.value), new Date())}
          </div>
        </Tooltip>
      ),
    },
    {
      field: 'updated_at',
      hide: true,
      valueGetter: (value) => {
        return value.row.updated_at
      },
      headerName: 'Updated At',
      flex: 5,
      renderCell: (param) => (
        <div className="test-name">
          <Tooltip title={param.value}>
            <span>{relativeTime(new Date(param.value), new Date())}</span>
          </Tooltip>
        </div>
      ),
    },
    {
      field: 'details',
      valueGetter: (value) => {
        return value.row.id
      },
      headerName: 'Details',
      flex: 2,
      renderCell: (param) => (
        <a
          href={'/sippy-ng/triages/' + param.value}
          target="_blank"
          rel="noopener noreferrer"
        >
          <InfoIcon />
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
        selectionModel={activeRow}
        onSelectionModelChange={handleSetSelectionModel}
        components={{ Toolbar: GridToolbar }}
        rows={triageEntries}
        columns={columns}
        getRowId={(row) => String(row.id)}
        pageSize={entriesPerPage}
        page={activePage}
        onPageChange={(newPage) => {
          setActivePage(newPage, 'replaceIn')
        }}
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
  entriesPerPage: PropTypes.number,
  allRegressedTests: PropTypes.array,
}
