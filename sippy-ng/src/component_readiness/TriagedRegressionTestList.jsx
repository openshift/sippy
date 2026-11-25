import { ageColor, relativeTime, useFilterModelParam } from '../helpers'
import { applyFilterModel, shouldKeepFilterItem } from '../datagrid/filterUtils'
import { Badge, Box, Tooltip, Typography } from '@mui/material'
import {
  CellKind,
  triagedRegressionCellRank,
  triagedRegressionViewCell,
  triagedRegressionViewNames,
} from './HistoricalRegressionUtils'
import { CheckCircle, History, HistoryToggleOff } from '@mui/icons-material'
import { DataGrid } from '@mui/x-data-grid'
import { filterRegressionsByLabel } from './TriageSymptomLabels'
import { Link } from 'react-router'
import { NumberParam, useQueryParam } from 'use-query-params'
import { pathForExactTestAnalysisWithVariants } from '../helpers'
import { styled, useTheme } from '@mui/material/styles'
import CompSeverityIcon from './CompSeverityIcon'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

const StatusCell = styled('div')({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
})

// The green check marks closure of the tracked regression (or of this view's
// association), not a passing assessment for the historical window.
function HistoricalIcon() {
  return (
    <Badge
      overlap="circular"
      anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      badgeContent={
        <CheckCircle
          color="success"
          sx={{
            fontSize: 14,
            bgcolor: 'background.paper',
            borderRadius: '50%',
          }}
        />
      }
    >
      <History color="action" />
    </Badge>
  )
}

// HistoricalStatusCell links a closed regression to its historical report, or
// explains why no historical report is available. tabIndex is the grid's
// managed value, so arrow-key navigation focuses the control and inactive
// cells stay out of the tab sequence.
function HistoricalStatusCell({ cell, tabIndex }) {
  if (cell.kind === CellKind.UNAVAILABLE) {
    return (
      <Tooltip title={cell.tooltip}>
        <span
          role="img"
          aria-label={cell.tooltip}
          tabIndex={tabIndex}
          className="historical-unavailable"
        >
          <HistoryToggleOff color="disabled" />
        </span>
      </Tooltip>
    )
  }
  return (
    <Tooltip title={cell.tooltip}>
      <a
        href={cell.url}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={cell.tooltip}
        tabIndex={tabIndex}
        className="historical-test-details"
        onClick={(e) => e.stopPropagation()}
      >
        <HistoricalIcon />
      </a>
    </Tooltip>
  )
}

HistoricalStatusCell.propTypes = {
  cell: PropTypes.object.isRequired,
  tabIndex: PropTypes.number,
}

export default function TriagedRegressionTestList(props) {
  const theme = useTheme()
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
  const [filterModel, setFilterModel] = useFilterModelParam(
    'regressedModalTestFilters'
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

  const [triagedRegressions, setTriagedRegressions] = React.useState([])
  const [showView, setShowView] = React.useState(false)

  React.useEffect(() => {
    // undefined means the parent has not selected a triage yet, so keep a list
    // an event already filled. null is an empty triage (mass triage).
    if (props.regressions === undefined) {
      return
    }
    const regressions = props.regressions ?? []
    setTriagedRegressions(regressions)
    setShowView(regressions.length > 0)
  }, [props.regressions])

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
  // Columns come from current reports and from views where a listed regression
  // closed, so closed regressions keep a link after rolling off the report.
  const viewNames = triagedRegressionViewNames(
    regressedTestsByView,
    triagedRegressions
  )
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
      renderCell: (params) => (
        <Link
          to={pathForExactTestAnalysisWithVariants(
            params.row.release,
            params.row.test_name,
            params.row.variants
          )}
          className="test-name"
          onClick={(e) => e.stopPropagation()}
        >
          {params.value}
        </Link>
      ),
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
      renderCell: (param) => {
        if (!param.value) return ''
        const color = ageColor(param.value, theme)
        return (
          <Tooltip title="WARNING: This is the first time we detected this test regressed in the default query. This value is not relevant if you've altered query parameters from the default.">
            <div
              className="regressed-since"
              style={color ? { color } : undefined}
            >
              {relativeTime(param.value, new Date())}
            </div>
          </Tooltip>
        )
      },
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
    {
      field: 'force_closed',
      headerName: 'Force Closed',
      flex: 8,
      filterable: true,
      sortable: false,
      valueGetter: (value, row) => (row.force_closed ? 'Yes' : 'No'),
      renderCell: (params) => {
        if (!params.row.force_closed) {
          return null
        }
        const details = [
          params.row.force_closed_reason
            ? `Reason: ${params.row.force_closed_reason}`
            : null,
          params.row.force_closed_by
            ? `By: ${params.row.force_closed_by}`
            : null,
          params.row.force_closed_by_triage_id
            ? `Triage: ${params.row.force_closed_by_triage_id}`
            : null,
        ]
          .filter(Boolean)
          .join('\n')
        return (
          <Tooltip
            title={
              <Box component="span" sx={{ whiteSpace: 'pre-line' }}>
                {details || 'Force closed'}
              </Box>
            }
          >
            <CheckCircle color="success" />
          </Tooltip>
        )
      },
    },
    ...(showStatus
      ? viewNames.map((viewName) => {
          // Keyed by view name so a saved sort stays on the same view when a
          // historical column is added or the columns reorder.
          const field = `status:${viewName}`
          return {
            field,
            headerName: viewName,
            filterable: false,
            renderHeader: () => (
              <Tooltip
                title={`${viewName}: status for this view (base vs sample). Closed regressions link to a historical report ending on their last failure date.`}
              >
                <span
                  style={{
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {viewName}
                </span>
              </Tooltip>
            ),
            valueGetter: (value, row) =>
              triagedRegressionViewCell(row, viewName, regressedTestsByView),
            sortComparator: (a, b) =>
              triagedRegressionCellRank(a) - triagedRegressionCellRank(b),
            renderCell: (params) => {
              const cell = params.value
              if (cell == null) return null
              if (cell.kind !== CellKind.CURRENT) {
                return (
                  <StatusCell className="status">
                    <HistoricalStatusCell
                      cell={cell}
                      tabIndex={params.tabIndex}
                    />
                  </StatusCell>
                )
              }
              const icon = (
                <CompSeverityIcon
                  status={cell.status}
                  explanations={cell.explanations}
                />
              )
              return (
                <StatusCell className="status">
                  {cell.url ? (
                    <a
                      href={cell.url}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {icon}
                    </a>
                  ) : (
                    icon
                  )}
                </StatusCell>
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
      {showView && (
        <div className="cr-triage-panel-element">
          <Typography>Test Failures</Typography>
          <DataGrid
            sortModel={sortModel}
            onSortModelChange={setSortModel}
            slots={{ toolbar: GridToolbar }}
            rows={filteredRegressions}
            columns={columns}
            getRowHeight={() => 'auto'}
            getRowId={(row) => row.id}
            rowSelectionModel={activeRow != null ? [activeRow] : []}
            onRowSelectionModelChange={(newRow) => {
              if (newRow.length > 0) {
                setActiveRow(Number(newRow[0]), 'replaceIn')
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
      )}
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
