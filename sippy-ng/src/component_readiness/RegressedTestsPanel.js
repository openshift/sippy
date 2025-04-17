import { CapabilitiesContext } from '../App'
import { CompReadyVarsContext } from './CompReadyVars'
import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { FileCopy } from '@mui/icons-material'
import {
  formColumnName,
  getTriagesAPIUrl,
  sortQueryParams,
} from './CompReadyUtils'
import {
  FormHelperText,
  MenuItem,
  Popover,
  Select,
  Snackbar,
  TextField,
  Tooltip,
} from '@mui/material'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { relativeTime, safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/material/Alert'
import Button from '@mui/material/Button'
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
  testName,
  testBasisRelease
) {
  const environmentVal = formColumnName({ variants: variants })
  const { expandEnvironment } = useContext(CompReadyVarsContext)
  const safeComponentName = safeEncodeURIComponent(componentName)
  const safeTestId = safeEncodeURIComponent(testId)
  const safeTestName = safeEncodeURIComponent(testName)
  const safeTestBasisRelease = safeEncodeURIComponent(testBasisRelease)
  let variantsUrl = ''
  Object.entries(variants).forEach(([key, value]) => {
    variantsUrl += '&' + key + '=' + safeEncodeURIComponent(value)
  })
  const retUrl =
    '/component_readiness/test_details' +
    filterVals +
    `&testBasisRelease=${safeTestBasisRelease}` +
    `&testId=${safeTestId}` +
    expandEnvironment(environmentVal) +
    `&component=${safeComponentName}` +
    `&capability=${capabilityName}` +
    variantsUrl +
    `&testName=${safeTestName}`

  return sortQueryParams(retUrl)
}

const useStyles = makeStyles({
  triageForm: {
    display: 'flex',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 16,
    padding: '10px 0',
  },
  validationErrors: {
    color: 'red',
  },
})

export default function RegressedTestsPanel(props) {
  const { filterVals, regressedTests, setTriageEntryCreated } = props
  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

  const classes = useStyles()

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
  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const triageEnabled = capabilitiesContext.includes('write_endpoints')
  const [triaging, setTriaging] = React.useState(false)
  const [triageEntryData, setTriageEntryData] = React.useState({
    url: '',
    type: 'type',
    ids: [],
  })
  const [triageValidationErrors, setTriageValidationErrors] = React.useState([])
  const jiraUrlPrefix = 'https://issues.redhat.com/browse'

  const handleTriageChange = (e) => {
    const { name, value, checked } = e.target

    // The checkboxes require special handling to keep in sync
    if (name === 'triage-test-id') {
      if (checked) {
        setTriageEntryData((prevData) => ({
          ...prevData,
          ids: [...prevData.ids, value],
        }))
      } else {
        setTriageEntryData((prevData) => ({
          ...prevData,
          ids: prevData.ids.filter((id) => id !== value),
        }))
      }
    } else {
      setTriageEntryData((prevData) => ({
        ...prevData,
        [name]: value,
      }))
    }
  }

  const handleTriageEntrySubmit = () => {
    const validationErrors = []
    if (triageEntryData.type === 'type') {
      validationErrors.push('invalid type, please make a selection')
    }
    if (!triageEntryData.url.startsWith(jiraUrlPrefix)) {
      validationErrors.push('invalid url, should begin with ' + jiraUrlPrefix)
    }
    if (triageEntryData.ids.length < 1) {
      validationErrors.push('no tests selected, please select at least one')
    }
    setTriageValidationErrors(validationErrors)

    if (validationErrors.length === 0) {
      const data = {
        url: triageEntryData.url,
        type: triageEntryData.type,
        regressions: triageEntryData.ids.map((id) => {
          return { id: Number(id) }
        }),
      }

      fetch(getTriagesAPIUrl(), {
        method: 'POST',
        body: JSON.stringify(data),
      }).then((response) => {
        if (!response.ok) {
          response.json().then((data) => {
            let errorMessage = 'invalid response returned from server'
            if (data?.code) {
              errorMessage =
                'error creating triage entry: ' +
                data.code +
                ': ' +
                data.message
            }
            console.error(errorMessage)
            setAlertText(errorMessage)
            setAlertSeverity('error')
          })
          return
        }

        setTriageEntryCreated(true)
        setAlertText('successfully created triage entry')
        setAlertSeverity('success')
        setTriaging(false)
        setTriageEntryData({
          url: '',
          type: 'type',
          ids: [],
        })
      })
    }
  }

  const [alertText, setAlertText] = React.useState('')
  const [alertSeverity, setAlertSeverity] = React.useState('')
  const handleAlertClose = (event, reason) => {
    if (reason === 'clickaway') {
      return
    }
    setAlertText('')
    setAlertSeverity('')
  }

  const triageTypeOptions = [
    'type',
    'ci-infra',
    'product-infra',
    'product',
    'test',
  ]

  // define table columns
  const columns = [
    ...(triaging
      ? [
          {
            field: 'triage',
            headerName: 'Triage',
            flex: 4,
            valueGetter: (params) => {
              return String(params.row.regression_id)
            },
            renderCell: (param) => (
              <input
                type="checkbox"
                name="triage-test-id"
                value={param.value}
                onChange={handleTriageChange}
                checked={triageEntryData.ids.includes(param.value)}
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
      field: 'opened',
      headerName: 'Regressed Since',
      flex: 12,
      valueGetter: (params) => {
        if (!params.row.opened) {
          // For a regression we haven't yet detected:
          return null
        }
        return new Date(params.row.opened).getTime()
      },
      renderCell: (params) => {
        if (!params.value) return ''
        const regressedSinceDate = new Date(params.value)
        return (
          <Tooltip
            title={`WARNING: This is the first time we detected this test regressed in the default query. This value is not relevant if you've altered query parameters from the default. 
            Click to copy the regression ID (${params.row.regression_id}) if one is defined. Useful for triage.`}
            onClick={(event) =>
              copyTestToClipboard(event, params.row.regression_id)
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
          <Link
            to={generateTestReport(
              params.row.test_id,
              params.row.variants,
              filterVals,
              params.row.component,
              params.row.capability,
              params.row.test_name,
              params.row.base_stats ? params.row.base_stats.release : ''
            )}
          >
            <CompSeverityIcon
              status={
                params.row.effective_status
                  ? params.row.effective_status
                  : params.row.status
              }
              explanations={params.row.explanations}
            />
          </Link>
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
      {triaging ? (
        <div className={classes.triageForm}>
          <TextField
            name="url"
            label="Jira URL"
            value={triageEntryData.url}
            onChange={handleTriageChange}
          />
          <Select
            name="type"
            label="Type"
            value={triageEntryData.type}
            onChange={handleTriageChange}
          >
            {triageTypeOptions.map((option, index) => (
              <MenuItem key={index} value={option}>
                {option}
              </MenuItem>
            ))}
          </Select>
          <Button
            variant="contained"
            color="primary"
            onClick={handleTriageEntrySubmit}
          >
            Create Entry
          </Button>
          {triageValidationErrors && (
            <FormHelperText className={classes.validationErrors}>
              {triageValidationErrors.map((text, index) => (
                <span key={index}>
                  {text}
                  <br />
                </span>
              ))}
            </FormHelperText>
          )}
        </div>
      ) : null}
      {triageEnabled ? (
        <Button
          variant="contained"
          color="secondary"
          onClick={() => setTriaging(!triaging)}
        >
          {triaging ? 'Close' : 'Triage'}
        </Button>
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
  setTriageEntryCreated: PropTypes.func,
  filterVals: PropTypes.string.isRequired,
}
