import { applyFilterModel } from '../datagrid/filterUtils'
import { CheckCircle, Error as ErrorIcon } from '@mui/icons-material'
import { DataGrid } from '@mui/x-data-grid'
import { formatDateToSeconds, relativeTime, SafeJSONParam } from '../helpers'
import { hasFailedFixRegression, jiraUrlPrefix } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { Tooltip, Typography } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import CompSeverityIcon from './CompSeverityIcon'
import GridToolbar from '../datagrid/GridToolbar'
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
  const [filterModel = { items: [] }, setFilterModel] = useQueryParam(
    'triageFilters',
    SafeJSONParam,
    { updateType: 'replaceIn' }
  )

  const addFilters = (filter) => {
    const currentFilters = filterModel.items.filter((item) => item.value !== '')

    filter.forEach((item) => {
      if (item.value && item.value !== '') {
        currentFilters.push(item)
      }
    })
    setFilterModel({
      items: currentFilters,
      linkOperator: filterModel.linkOperator || 'and',
    })
  }

  // Quick search functionality - searches description field
  const requestSearch = (searchValue) => {
    // Filter out empty items and existing description filters
    const currentFilters = filterModel.items.filter(
      (f) => f.value !== '' && f.columnField !== 'description'
    )
    if (searchValue && searchValue !== '') {
      currentFilters.push({
        id: 99,
        columnField: 'description',
        operatorValue: 'contains',
        value: searchValue,
      })
    }
    setFilterModel({
      items: currentFilters,
      linkOperator: filterModel.linkOperator || 'and',
    })
  }

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

  const columns = [
    {
      field: 'resolution_date',
      valueGetter: (value) => {
        return value.row.resolved?.Valid ? value.row.resolved.Time : ''
      },
      headerName: 'Resolved',
      flex: 4,
      align: 'center',
      filterable: false,
      renderCell: (param) => {
        const triage = triageEntries.find((t) => t.id === param.row.id)
        const hasFailedFix = hasFailedFixRegression(triage, allRegressedTests)

        return param.value ? (
          <Tooltip
            title={`${relativeTime(
              new Date(param.value),
              new Date()
            )} (${formatDateToSeconds(param.value)})`}
          >
            {hasFailedFix ? (
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
      autocomplete: 'type',
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
      filterable: false,
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
      autocomplete: 'bug_state',
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
      autocomplete: 'bug_version',
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'release_blocker',
      valueGetter: (value) => {
        return value.row.bug?.release_blocker || ''
      },
      headerName: 'Release Blocker',
      flex: 5,
      autocomplete: 'release_blocker',
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'last_change',
      valueGetter: (value) => {
        return value.row.bug?.last_change_time || ''
      },
      headerName: 'Jira updated',
      flex: 5,
      filterable: false,
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
      filterable: false,
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
      filterable: false,
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
      filterable: false,
      renderCell: (param) => {
        const triageUrl = '/component_readiness/triages/' + param.value
        return (
          <Link to={triageUrl} target="_blank" rel="noopener noreferrer">
            <InfoIcon />
          </Link>
        )
      },
    },
  ]

  // Apply client-side filtering using shared utility
  const filteredTriageEntries = React.useMemo(
    () => applyFilterModel(triageEntries, filterModel, columns),
    [triageEntries, filterModel, columns]
  )

  return (
    <Fragment>
      <Typography>Triaged Test Regressions</Typography>
      <DataGrid
        sortModel={sortModel}
        onSortModelChange={setSortModel}
        selectionModel={activeRow}
        onSelectionModelChange={handleSetSelectionModel}
        components={{ Toolbar: GridToolbar }}
        rows={filteredTriageEntries}
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
            addFilters: addFilters,
            filterModel: filterModel,
            setFilterModel: setFilterModel,
            clearSearch: () => requestSearch(''),
            doSearch: requestSearch,
            autocompleteData: triageEntries,
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
