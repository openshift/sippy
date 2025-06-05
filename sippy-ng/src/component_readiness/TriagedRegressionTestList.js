import { CompReadyVarsContext } from './CompReadyVars'
import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import {
  formColumnName,
  generateTestReportForRegressedTest,
} from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { NumberParam, useQueryParam } from 'use-query-params'
import { relativeTime } from '../helpers'
import { Tooltip, Typography } from '@mui/material'
import CompSeverityIcon from './CompSeverityIcon'
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

  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

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

  const showStatus =
    props.allRegressedTests !== undefined && props.allRegressedTests.length > 0

  const columns = [
    {
      field: 'test_name',
      headerName: 'Test Name',
      flex: 40,
      valueGetter: (params) => {
        return params.row.test_name
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'release',
      headerName: 'Release',
      flex: 20,
      valueGetter: (params) => {
        return params.row.release
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'variants',
      headerName: 'Variants',
      flex: 30,
      valueGetter: (params) => {
        return formColumnName({ variants: params.row.variants })
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'opened',
      headerName: 'Regressed Since',
      flex: 12,
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
      ? [
          {
            field: 'status',
            headerName: 'Status',
            valueGetter: (params) => {
              const value = {
                status: '',
                explanations: '',
                url: '',
              }
              const regressionId = params.row.id
              const matchingRegression = props.allRegressedTests.find(
                (rt) => rt.regression?.id === regressionId
              )
              if (matchingRegression) {
                value.status = matchingRegression.status
                value.explanations = matchingRegression.explanations
                value.url = generateTestReportForRegressedTest(
                  matchingRegression,
                  props.filterVals,
                  expandEnvironment
                )
              }
              return value
            },
            renderCell: (params) => (
              <div
                style={{
                  textAlign: 'center',
                }}
                className="status"
              >
                <Link to={params.value.url}>
                  <CompSeverityIcon
                    status={params.value.status}
                    explanations={params.value.explanations}
                  />
                </Link>
              </div>
            ),
            flex: 6,
          },
        ]
      : []),
  ]

  return (
    <Fragment>
      <div hidden={!showView} className="cr-triage-panel-element">
        <Typography>Test Failures</Typography>
        <DataGrid
          sortModel={sortModel}
          onSortModelChange={setSortModel}
          components={{ Toolbar: GridToolbar }}
          rows={triagedRegressions}
          columns={columns}
          getRowId={(row) => row.id}
          selectionModel={activeRow}
          onSelectionModelChange={(newRow) => {
            // Due to the usage of the eventEmitter, this can sometimes fire when we don't want it to actually de-select
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
  allRegressedTests: PropTypes.array,
  filterVals: PropTypes.string,
  showOnLoad: PropTypes.bool,
}
