import { CompReadyVarsContext } from './CompReadyVars'
import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { FileCopy } from '@mui/icons-material'
import { formColumnName, sortQueryParams } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { Popover, Tooltip } from '@mui/material'
import { relativeTime, safeEncodeURIComponent } from '../helpers'
import CompSeverityIcon from './CompSeverityIcon'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'

// Construct a URL with all existing filters plus testId, environment, and testName.
// This is the url used when you click inside a TableCell on page4 on the right.
// We pass these arguments to the component that generates the test details report.
function generateTestReport(
  testId,
  variants,
  filterVals,
  componentName,
  capabilityName,
  testName
) {
  const environmentVal = formColumnName({ variants: variants })
  const { expandEnvironment } = useContext(CompReadyVarsContext)
  const safeComponentName = safeEncodeURIComponent(componentName)
  const safeTestId = safeEncodeURIComponent(testId)
  const safeTestName = safeEncodeURIComponent(testName)
  let variantsUrl = ''
  Object.entries(variants).forEach(([key, value]) => {
    variantsUrl += '&' + key + '=' + safeEncodeURIComponent(value)
  })
  const retUrl =
    '/component_readiness/test_details' +
    filterVals +
    `&testId=${safeTestId}` +
    expandEnvironment(environmentVal) +
    `&component=${safeComponentName}` +
    `&capability=${capabilityName}` +
    variantsUrl +
    `&testName=${safeTestName}`

  return sortQueryParams(retUrl)
}

export default function RegressedTestsPanel(props) {
  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

  // Helpers for copying the test ID to clipboard
  const [copyPopoverEl, setCopyPopoverEl] = React.useState(null)
  const copyPopoverOpen = Boolean(copyPopoverEl)
  const copyTestID = (event, testId) => {
    event.preventDefault()
    navigator.clipboard.writeText(testId)
    setCopyPopoverEl(event.currentTarget)
    setTimeout(() => setCopyPopoverEl(null), 2000)
  }

  // define table columns
  const columns = [
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
      field: 'fisher_exact',
      headerName: 'Certainty',
      flex: 8,
      valueGetter: (params) => {
        if (!params.row.fisher_exact) {
          return ''
        }
        return (100 - params.row.fisher_exact * 100).toFixed(1)
      },
      renderCell: (param) => (
        <div className="fishers-exact">{param.value}%</div>
      ),
    },
    {
      field: 'pass_rate_delta',
      headerName: 'Pass Rate Delta',
      flex: 8,
      valueGetter: (params) => {
        if (!params.row.sample_stats || !params.row.base_stats) {
          return ''
        }
        return (
          (params.row.sample_stats.success_rate * 100).toFixed(0) -
          (params.row.base_stats.success_rate * 100).toFixed(0)
        )
      },
      renderCell: (param) => <div className="pass-rate">{param.value}%</div>,
    },
    {
      field: 'test_id',
      flex: 5,
      headerName: 'ID',
      renderCell: (params) => {
        return (
          <IconButton
            onClick={(event) => copyTestID(event, params.value)}
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
          <Link
            to={generateTestReport(
              params.row.test_id,
              params.row.variants,
              props.filterVals,
              params.row.component,
              params.row.capability,
              params.row.test_name
            )}
          >
            <CompSeverityIcon status={params.value} />
          </Link>
        </div>
      ),
      flex: 6,
    },
  ]

  return (
    <Fragment>
      <DataGrid
        sortModel={sortModel}
        onSortModelChange={setSortModel}
        components={{ Toolbar: GridToolbar }}
        rows={props.regressedTests}
        columns={columns}
        getRowId={(row) =>
          row.test_id +
          row.component +
          row.capability +
          Object.keys(row.variants)
            .map((key) => row.variants[key])
            .join(' ')
        }
        pageSize={10}
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
  filterVals: PropTypes.string.isRequired,
}
