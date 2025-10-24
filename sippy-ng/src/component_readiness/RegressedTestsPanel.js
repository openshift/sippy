import { CompReadyVarsContext } from './CompReadyVars'
import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { FileCopy } from '@mui/icons-material'
import { formColumnName, generateTestDetailsReportLink } from './CompReadyUtils'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { Popover, Snackbar, Tooltip } from '@mui/material'
import { relativeTime } from '../helpers'
import { SippyCapabilitiesContext } from '../App'
import Alert from '@mui/material/Alert'
import Button from '@mui/material/Button'
import CompSeverityIcon from './CompSeverityIcon'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import UpsertTriageModal from './UpsertTriageModal'

export default function RegressedTestsPanel(props) {
  const [activeRow, setActiveRow] = useQueryParam(
    'regressedModalRow',
    StringParam,
    { updateType: 'replaceIn' }
  )
  const [activePage, setActivePage] = useQueryParam(
    'regressedModalPage',
    NumberParam,
    { updateType: 'replaceIn' }
  )
  const { expandEnvironment, views, view } = useContext(CompReadyVarsContext)
  const { filterVals, regressedTests, setTriageActionTaken } = props
  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

  // Helpers for copying the test ID to clipboard
  const [copyPopoverEl, setCopyPopoverEl] = React.useState(null)
  const copyPopoverOpen = Boolean(copyPopoverEl)
  const copyTestToClipboard = (event, testId) => {
    event.preventDefault()
    navigator.clipboard.writeText(testId)
    setCopyPopoverEl(event.currentTarget)
    setTimeout(() => setCopyPopoverEl(null), 2000)
  }

  // Helpers to create triage entries
  const capabilitiesContext = React.useContext(SippyCapabilitiesContext)
  const triageEnabled = capabilitiesContext.includes('write_endpoints')

  const currentView = views.find((v) => v.name === view)
  const regressionTrackingEnabled =
    currentView?.regression_tracking?.enabled || false
  const triageButtonEnabled = triageEnabled && regressionTrackingEnabled

  const [triaging, setTriaging] = React.useState(false)
  const [regressionIds, setRegressionIds] = React.useState([])

  const handleTriageFormCompletion = () => {
    setRegressionIds([])
    setTriaging(false)
    setTriageActionTaken(true)
  }

  const handleTriageTestIdChange = (e) => {
    const { value, checked } = e.target
    if (checked) {
      setRegressionIds((prevData) => [...prevData, value])
    } else {
      setRegressionIds((prevData) => prevData.filter((id) => id !== value))
    }
  }

  const [alertText, setAlertText] = React.useState('')
  const [alertSeverity, setAlertSeverity] = React.useState('success')
  const handleAlertClose = (event, reason) => {
    if (reason === 'clickaway') {
      return
    }
    setAlertText('')
    setAlertSeverity('')
  }

  // define table columns
  const columns = [
    ...(triaging
      ? [
          {
            field: 'triage',
            headerName: 'Triage',
            flex: 4,
            valueGetter: (params) => {
              if (!params.row.regression?.opened) {
                // For a regression we haven't yet detected:
                return '0'
              }
              return String(params.row.regression.id)
            },
            renderCell: (param) => (
              <input
                type="checkbox"
                name="triage-test-id"
                value={param.value}
                onChange={handleTriageTestIdChange}
                checked={regressionIds.includes(param.value)}
                disabled={param.value === '0'}
              />
            ),
          },
        ]
      : []),
    {
      field: 'component',
      headerName: 'Component',
      flex: 20,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'capability',
      headerName: 'Capability',
      flex: 12,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'test_name',
      headerName: 'Test Name',
      flex: 40,
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'test_suite',
      headerName: 'Test Suite',
      flex: 15,
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
      field: 'regression',
      headerName: 'Regressed Since',
      flex: 12,
      valueGetter: (params) => {
        if (!params.row.regression?.opened) {
          // For a regression we haven't yet detected:
          return null
        }
        return new Date(params.row.regression.opened).getTime()
      },
      renderCell: (params) => {
        if (!params.value) return ''
        const regressedSinceDate = new Date(params.row.regression.opened)
        return (
          <Tooltip
            title={`WARNING: This is the first time we detected this test regressed in the default query. This value is not relevant if you've altered query parameters from the default. 
            Click to copy the regression ID (${params.row.regression.id}) if one is defined. Useful for triage.`}
            onClick={(event) =>
              copyTestToClipboard(event, params.row.regression.id)
            }
          >
            <div className="regressed-since">
              {relativeTime(regressedSinceDate, new Date())}
            </div>
          </Tooltip>
        )
      },
    },
    {
      field: 'last_failure',
      headerName: 'Last Failure',
      flex: 12,
      valueGetter: (params) => {
        if (!params.row.last_failure) {
          return null
        }
        return new Date(params.row.last_failure).getTime()
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
    {
      field: 'test_id',
      flex: 5,
      headerName: 'ID',
      renderCell: (params) => {
        return (
          <IconButton
            onClick={(event) => copyTestToClipboard(event, params.value)}
            size="small"
            aria-label="Copy test ID"
            color="inherit"
            sx={{ marginBottom: 1 }}
          >
            <Tooltip title="Copy test ID">
              <FileCopy color="primary" />
            </Tooltip>
          </IconButton>
        )
      },
    },
    {
      field: 'status',
      headerName: 'Status',
      renderCell: (params) => (
        <div
          style={{
            textAlign: 'center',
          }}
          className="status"
        >
          <a
            href={generateTestDetailsReportLink(
              params.row,
              filterVals,
              expandEnvironment
            )}
            target="_blank"
            rel="noopener noreferrer"
          >
            <CompSeverityIcon
              status={
                params.row.effective_status
                  ? params.row.effective_status
                  : params.row.status
              }
              explanations={params.row.explanations}
            />
          </a>
        </div>
      ),
      flex: 6,
    },
  ]

  return (
    <Fragment>
      <Snackbar
        open={alertText.length > 0}
        autoHideDuration={10000}
        onClose={handleAlertClose}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        <Alert onClose={handleAlertClose} severity={alertSeverity}>
          {alertText}
        </Alert>
      </Snackbar>
      <DataGrid
        sortModel={sortModel}
        onSortModelChange={setSortModel}
        components={{ Toolbar: GridToolbar }}
        rows={regressedTests}
        columns={columns}
        getRowId={(row) =>
          row.test_id +
          row.component +
          row.capability +
          Object.keys(row.variants)
            .map((key) => row.variants[key])
            .join(' ')
        }
        selectionModel={activeRow}
        onSelectionModelChange={(newRow) => {
          if (newRow.length > 0) {
            setActiveRow(String(newRow), 'replaceIn')
          }
        }}
        pageSize={10}
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
            showQuickFilter: true,
          },
        }}
      />
      {triaging && (
        <UpsertTriageModal
          regressionIds={regressionIds}
          buttonText="Set Triage Info"
          setComplete={handleTriageFormCompletion}
          submissionDelay={1500}
        />
      )}
      {triageEnabled ? (
        <Tooltip
          title={
            !regressionTrackingEnabled
              ? 'Triage is not available because regression tracking is not enabled for this view'
              : ''
          }
        >
          <span>
            <Button
              variant="contained"
              color="secondary"
              sx={'margin: 10px'}
              onClick={() => setTriaging(!triaging)}
              disabled={!triageButtonEnabled}
            >
              {triaging ? 'Cancel' : 'Triage'}
            </Button>
          </span>
        </Tooltip>
      ) : null}

      <Popover
        id="copyPopover"
        open={copyPopoverOpen}
        anchorEl={copyPopoverEl}
        onClose={() => setCopyPopoverEl(null)}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'center',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'center',
        }}
      >
        ID copied!
      </Popover>
    </Fragment>
  )
}

RegressedTestsPanel.propTypes = {
  regressedTests: PropTypes.array,
  setTriageActionTaken: PropTypes.func,
  filterVals: PropTypes.string.isRequired,
}
