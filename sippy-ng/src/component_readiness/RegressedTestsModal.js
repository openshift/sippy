import { Button, Grid, Typography } from '@mui/material'
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
  platform,
  upgrade,
  arch,
  network,
  variant,
  filterVals,
  componentName,
  capabilityName,
  testName
) {
  const environment = {
    network: network,
    upgrade: upgrade,
    arch: arch,
    platform: platform,
    variant: variant,
  }
  const environmentVal = formColumnName(environment)
  const { expandEnvironment } = useContext(CompReadyVarsContext)
  const safeComponentName = safeEncodeURIComponent(componentName)
  const safeTestId = safeEncodeURIComponent(testId)
  const safeTestName = safeEncodeURIComponent(testName)
  const safePlatform = safeEncodeURIComponent(platform)
  const safeUpgrade = safeEncodeURIComponent(upgrade)
  const safeArch = safeEncodeURIComponent(arch)
  const safeNetwork = safeEncodeURIComponent(network)
  const safeVariant = safeEncodeURIComponent(variant)
  const retUrl =
    '/component_readiness/test_details' +
    filterVals +
    `&testId=${safeTestId}` +
    expandEnvironment(environmentVal) +
    `&component=${safeComponentName}` +
    `&capability=${capabilityName}` +
    `&platform=${safePlatform}` +
    `&upgrade=${safeUpgrade}` +
    `&arch=${safeArch}` +
    `&network=${safeNetwork}` +
    `&variant=${safeVariant}` +
    `&testName=${safeTestName}`

  return sortQueryParams(retUrl)
}

export default function RegressedTestsModal(props) {
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
      field: 'network',
      headerName: 'Network',
      flex: 6,
    },
    {
      field: 'upgrade',
      headerName: 'Upgrade',
      flex: 10,
    },
    {
      field: 'arch',
      headerName: 'Arch',
      flex: 6,
    },
    {
      field: 'platform',
      headerName: 'Platform',
      flex: 6,
    },
    {
      field: 'variant',
      headerName: 'Variant',
      flex: 8,
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
              params.row.platform,
              params.row.upgrade,
              params.row.arch,
              params.row.network,
              params.row.variant,
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
            components={{ Toolbar: GridToolbar }}
            rows={props.regressedTests}
            columns={columns}
            getRowId={(row) =>
              row.test_id +
              row.component +
              row.capability +
              row.variant +
              row.platform +
              row.network +
              row.arch +
              row.upgrade
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
