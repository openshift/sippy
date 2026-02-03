import { applyFilterModel, shouldKeepFilterItem } from '../datagrid/filterUtils'
import { CompReadyVarsContext } from './CompReadyVars'
import { DataGrid } from '@mui/x-data-grid'
import { generateTestDetailsReportLink } from './CompReadyUtils'
import { NumberParam, useQueryParam } from 'use-query-params'
import { relativeTime, SafeJSONParam } from '../helpers'
import { Tooltip, Typography } from '@mui/material'
import CompSeverityIcon from './CompSeverityIcon'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'

export default function TriagedRegressionTestList(props) {
  const { expandEnvironment } = useContext(CompReadyVarsContext)

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
      linkOperator: filterModel.linkOperator || 'and',
    })
  }

  // Quick search functionality - searches test_name field
  const requestSearch = (searchValue) => {
    // Filter out empty items and existing test_name filters
    const currentFilters = filterModel.items.filter(
      (f) => shouldKeepFilterItem(f) && f.columnField !== 'test_name'
    )
    if (searchValue && searchValue !== '') {
      currentFilters.push({
        id: 99,
        columnField: 'test_name',
        operatorValue: 'contains',
        value: searchValue,
      })
    }
    setFilterModel({
      items: currentFilters,
      linkOperator: filterModel.linkOperator || 'and',
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
      field: 'test_name',
      headerName: 'Test Name',
      flex: 50,
      autocomplete: 'test_name',
      valueGetter: (params) => {
        return params.row.test_name
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'release',
      headerName: 'Release',
      flex: 7,
      autocomplete: 'release',
      valueGetter: (params) => {
        return params.row.release
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'variants',
      headerName: 'Variants',
      flex: 20,
      valueGetter: (params) => {
        // Join array values into a searchable string
        return params.row.variants && Array.isArray(params.row.variants)
          ? params.row.variants.sort().join(' ')
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
      flex: 12,
      filterable: false,
      valueGetter: (params) => {
        if (!params.row.opened) {
          // For a regression we haven't yet detected:
          return ''
        }
        const regressedSinceDate = new Date(params.row.opened)
        return relativeTime(regressedSinceDate, new Date())
      },
      renderCell: (param) => (
        <Tooltip title="WARNING: This is the first time we detected this test regressed in the default query. This value is not relevant if you've altered query parameters from the default.">
          <div className="regressed-since">{param.value}</div>
        </Tooltip>
      ),
    },
    {
      field: 'last_failure',
      headerName: 'Last Failure',
      flex: 12,
      filterable: false,
      valueGetter: (params) => {
        if (!params.row.last_failure.Valid) {
          return null
        }
        return new Date(params.row.last_failure.Time).getTime()
      },
      renderCell: (params) => {
        if (!params.value) return ''
        const lastFailureDate = new Date(params.value)
        return (
          <div className="last-failure">
            {relativeTime(lastFailureDate, new Date())}
          </div>
        )
      },
    },
    ...(showStatus
      ? viewNames.map((viewName) => {
          const field = `status_${viewName.replace(/[^a-zA-Z0-9_-]/g, '_')}`
          return {
            field,
            headerName: viewName,
            filterable: false,
            renderHeader: () => (
              <Tooltip title="Status for this view (base vs sample). Only available when the regression has not rolled off the reporting window.">
                <span>{viewName}</span>
              </Tooltip>
            ),
            valueGetter: (params) => {
              const tests = regressedTestsByView[viewName] || []
              const rt = tests.find((t) => t?.regression?.id === params.row.id)
              if (!rt) return null
              return {
                status: rt.status,
                explanations: rt.explanations,
                url: generateTestDetailsReportLink(
                  rt,
                  props.filterVals,
                  expandEnvironment
                ),
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
                  <a href={item.url} target="_blank" rel="noopener noreferrer">
                    <CompSeverityIcon
                      status={item.status}
                      explanations={item.explanations}
                    />
                  </a>
                </div>
              )
            },
            flex: 6,
          }
        })
      : []),
  ]

  // Apply client-side filtering using shared utility
  const filteredRegressions = React.useMemo(
    () => applyFilterModel(triagedRegressions, filterModel, columns),
    [triagedRegressions, filterModel, columns]
  )

  return (
    <Fragment>
      <div hidden={!showView} className="cr-triage-panel-element">
        <Typography>Test Failures</Typography>
        <DataGrid
          sortModel={sortModel}
          onSortModelChange={setSortModel}
          components={{ Toolbar: GridToolbar }}
          rows={filteredRegressions}
          columns={columns}
          getRowHeight={() => 'auto'}
          getRowId={(row) => row.id}
          selectionModel={activeRow}
          onSelectionModelChange={(newRow) => {
            if (newRow.length > 0) {
              setActiveRow(Number(newRow), 'replaceIn')
            }
          }}
          page={activePage}
          onPageChange={(newPage) => {
            setActivePage(newPage, 'replaceIn')
          }}
          pageSize={10}
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
  regressions: PropTypes.array,
  allRegressedTests: PropTypes.object,
  filterVals: PropTypes.string,
  showOnLoad: PropTypes.bool,
}
