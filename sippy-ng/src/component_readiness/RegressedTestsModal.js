import { Button, Grid, Tooltip, Typography } from '@mui/material'
import { CompReadyVarsContext } from './CompReadyVars'
import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { formColumnName, sortQueryParams } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import CompSeverityIcon from './CompSeverityIcon'
import Dialog from '@mui/material/Dialog'
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

export default function RegressedTestsModal(props) {
  const [sortModel, setSortModel] = React.useState([
    { field: 'component', sort: 'asc' },
  ])

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
      renderCell: (param) => (
        <div className="test-name">
          {formColumnName({ variants: param.value })}
        </div>
      ),
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
      <Dialog
        fullWidth={true}
        maxWidth="xl"
        open={props.isOpen}
        onClose={props.close}
      >
        <Grid className="regressed-tests-dialog">
          <Typography
            variant="h6"
            style={{ marginTop: 20, marginBottom: 20, marginLeft: 20 }}
          >
            Regressed Tests
          </Typography>
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

          <Button
            style={{ marginTop: 20, marginBottom: 20, marginLeft: 20 }}
            variant="contained"
            color="primary"
            onClick={props.close}
          >
            CLOSE
          </Button>
        </Grid>
      </Dialog>
    </Fragment>
  )
}

RegressedTestsModal.propTypes = {
  regressedTests: PropTypes.array,
  filterVals: PropTypes.string.isRequired,
  isOpen: PropTypes.bool,
  close: PropTypes.func,
}
