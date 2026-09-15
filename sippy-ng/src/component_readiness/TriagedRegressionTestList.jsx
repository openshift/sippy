import { applyFilterModel, shouldKeepFilterItem } from '../datagrid/filterUtils'
import { DataGrid } from '@mui/x-data-grid'
import { filterRegressionsByLabel } from './TriageSymptomLabels'
import { generateTestDetailsReportLink } from './CompReadyUtils'
import { NumberParam, useQueryParam } from 'use-query-params'
import { relativeTime, SafeJSONParam } from '../helpers'
import { Tooltip, Typography } from '@mui/material'
import CompSeverityIcon from './CompSeverityIcon'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function TriagedRegressionTestList(props) {
  const [activeRow, setActiveRow] = useQueryParam(
    'regressedModalTestRow',
    NumberParam,
    { updateType: 'replaceIn' }
  )
  const [activePage, setActivePage] = useQueryParam(
    'regressedModalTestPage',
    NumberParam,
    { updateType: 'replaceIn' }
  )
  const [filterModel = { items: [] }, setFilterModel] = useQueryParam(
    'regressedModalTestFilters',
    SafeJSONParam,
    { updateType: 'replaceIn' }
  )

  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

  const addFilters = (filter) => {
    const currentFilters = filterModel.items.filter(shouldKeepFilterItem)

    filter.forEach((item) => {
      if (shouldKeepFilterItem(item)) {
        currentFilters.push(item)
      }
    })
    setFilterModel({
      items: currentFilters,
      logicOperator: filterModel.logicOperator || 'and',
    })
  }

  // Quick search functionality - searches test_name field
  const requestSearch = (searchValue) => {
    // Filter out empty items and existing test_name filters
    const currentFilters = filterModel.items.filter(
      (f) => shouldKeepFilterItem(f) && f.field !== 'test_name'
    )
    if (searchValue && searchValue !== '') {
      currentFilters.push({
        id: 99,
        field: 'test_name',
        operator: 'contains',
        value: searchValue,
      })
    }
    setFilterModel({
      items: currentFilters,
      logicOperator: filterModel.logicOperator || 'and',
    })
  }

  const [triagedRegressions, setTriagedRegressions] = React.useState(
    props.regressions !== undefined ? props.regressions : []
  )
  const [showView, setShowView] = React.useState(
    props.regressions !== undefined && props.regressions.length > 0
  )

  const handleTriagedRegressionGroupSelectionChanged = (data) => {
    let displayView = false
    if (data) {
      displayView = true
      setTriagedRegressions(data.regressions)
      setActiveRow(data.activeId, 'replaceIn')
    }

    setShowView(displayView)
  }
  if (props.eventEmitter !== undefined) {
    props.eventEmitter.on(
      'triagedEntrySelectionChanged',
      handleTriagedRegressionGroupSelectionChanged
    )
  }

  const regressedTestsByView = props.allRegressedTests || {}
  // Sort view names to ensure the main view is first
  const viewNames = [...Object.keys(regressedTestsByView)].sort((a, b) => {
    const aMain = a.endsWith('-main')
    const bMain = b.endsWith('-main')
    if (aMain && !bMain) return -1
    if (!aMain && bMain) return 1
    return a.localeCompare(b)
  })
  const showStatus = viewNames.length > 0

  const columns = [
    {
      field: 'id',
      headerName: 'ID',
      flex: 3,
      filterable: false,
      renderCell: (param) => <div>{param.value}</div>,
    },
    {
      field: 'test_name',
      headerName: 'Test Name',
      flex: 50,
      autocomplete: 'test_name',
      valueGetter: (value, row) => {
        return row.test_name
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'release',
      headerName: 'Release',
      flex: 7,
      autocomplete: 'release',
      valueGetter: (value, row) => {
        return row.release
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'variants',
      headerName: 'Variants',
      flex: 20,
      valueGetter: (value, row) => {
        // Join array values into a searchable string
        return row.variants && Array.isArray(row.variants)
          ? row.variants.sort().join(' ')
          : ''
      },
      renderCell: (params) => (
        <div className="variants-list">
          {params.value ? params.value.split(' ').join('\n') : ''}
        </div>
      ),
    },
    {
      field: 'opened',
      headerName: 'Regressed Since',
      type: 'date',
      flex: 12,
      filterable: false,
      valueGetter: (value, row) => {
        if (!row.opened) {
          return null
        }
        return new Date(row.opened)
      },
      renderCell: (param) => (
        <Tooltip title="WARNING: This is the first time we detected this test regressed in the default query. This value is not relevant if you've altered query parameters from the default.">
          <div className="regressed-since">
            {param.value ? relativeTime(param.value, new Date()) : ''}
          </div>
        </Tooltip>
      ),
    },
    {
      field: 'last_failure',
      headerName: 'Last Failure',
      flex: 12,
      filterable: false,
      type: 'date',
      valueGetter: (value, row) => {
        if (!row.last_failure.Valid) {
          return null
        }
        return new Date(row.last_failure.Time)
      },
      renderCell: (params) => {
        if (!params.value) return ''
        const lastFailureDate = params.value
        return (
          <div className="last-failure">
            {relativeTime(lastFailureDate, new Date())}
          </div>
        )
      },
    },
    ...(showStatus
      ? viewNames.map((viewName, index) => {
          const field = `status_${index}`
          return {
            field,
            headerName: viewName,
            filterable: false,
            renderHeader: () => (
              <Tooltip title="Status for this view (base vs sample). Only available when the regression has not rolled off the reporting window.">
                <span>{viewName}</span>
              </Tooltip>
            ),
            valueGetter: (value, row) => {
              const tests = regressedTestsByView[viewName] || []
              const rt = tests.find((t) => t?.regression?.id === row.id)
              if (!rt) return null
              return {
                status: rt.status,
                explanations: rt.explanations,
                url: generateTestDetailsReportLink(rt, viewName),
              }
            },
            renderCell: (params) => {
              if (params.value == null) return null
              const item = params.value
              return (
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                  className="status"
                >
                  {item.url ? (
                    <a
                      href={item.url}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <CompSeverityIcon
                        status={item.status}
                        explanations={item.explanations}
                      />
                    </a>
                  ) : (
                    <CompSeverityIcon
                      status={item.status}
                      explanations={item.explanations}
                    />
                  )}
                </div>
              )
            },
            flex: 6,
          }
        })
      : []),
  ]

  const labelFilteredRegressions = React.useMemo(
    () =>
      filterRegressionsByLabel(
        triagedRegressions,
        props.labelFilter,
        props.labelSummaries
      ),
    [triagedRegressions, props.labelFilter, props.labelSummaries]
  )

  // Apply client-side filtering using shared utility
  const filteredRegressions = React.useMemo(
    () => applyFilterModel(labelFilteredRegressions, filterModel, columns),
    [labelFilteredRegressions, filterModel, columns]
  )

  return (
    <Fragment>
      <div hidden={!showView} className="cr-triage-panel-element">
        <Typography>Test Failures</Typography>
        <DataGrid
          sortModel={sortModel}
          onSortModelChange={setSortModel}
          slots={{ toolbar: GridToolbar }}
          rows={filteredRegressions}
          columns={columns}
          getRowHeight={() => 'auto'}
          getRowId={(row) => row.id}
          rowSelectionModel={activeRow}
          onRowSelectionModelChange={(newRow) => {
            if (newRow.length > 0) {
              setActiveRow(Number(newRow), 'replaceIn')
            }
          }}
          paginationModel={{ pageSize: 10, page: activePage || 0 }}
          onPaginationModelChange={(model) => {
            setActivePage(model.page, 'replaceIn')
          }}
          rowHeight={60}
          autoHeight={true}
          checkboxSelection={false}
          slotProps={{
            toolbar: {
              columns: columns,
              addFilters: addFilters,
              filterModel: filterModel,
              setFilterModel: setFilterModel,
              clearSearch: () => requestSearch(''),
              doSearch: requestSearch,
              searchField: 'test_name',
              autocompleteData: triagedRegressions,
              downloadDataFunc: () => {
                return filteredRegressions
              },
              downloadFilePrefix: 'triaged_test_regressions',
            },
          }}
        />
      </div>
    </Fragment>
  )
}

TriagedRegressionTestList.propTypes = {
  eventEmitter: PropTypes.object,
  labelFilter: PropTypes.string,
  labelSummaries: PropTypes.array,
  regressions: PropTypes.array,
  allRegressedTests: PropTypes.object,
  showOnLoad: PropTypes.bool,
}
