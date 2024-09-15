import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { generateTestReport } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@mui/icons-material/HelpOutline'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import TableCell from '@mui/material/TableCell'

// CompReadyTestCall is for rendering the cells on the right of page4 or page4a
export default function CompReadyTestCell(props) {
  const {
    status,
    environment,
    testId,
    filterVals,
    component,
    capability,
    testName,
    regressedTests,
  } = props
  const theme = useTheme()
  const classes = useContext(ComponentReadinessStyleContext)

  const [componentParam, setComponentParam] = useQueryParam(
    'component',
    StringParam
  )
  const [capabilityParam, setCapabilityParam] = useQueryParam(
    'capability',
    StringParam
  )
  const [environmentParam, setEnvironmentParam] = useQueryParam(
    'environment',
    StringParam
  )
  const [testIdParam, setTestIdParam] = useQueryParam('testId', StringParam)
  const [testNameParam, setTestNameParam] = useQueryParam(
    'testName',
    StringParam
  )

  const { expandEnvironment } = useContext(CompReadyVarsContext)

  if (status === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          className={classes.crCellResult}
          style={{
            textAlign: 'center',
            backgroundColor: theme.palette.text.disabled,
          }}
        >
          <HelpOutlineIcon style={{ color: theme.palette.text.disabled }} />
        </TableCell>
      </Tooltip>
    )
  } else {
    return (
      <TableCell
        className={classes.crCellResult}
        style={{
          textAlign: 'center',
        }}
      >
        <Link
          to={generateTestReport(
            testId,
            expandEnvironment(environment),
            filterVals,
            component,
            capability,
            testName,
            regressedTests
          )}
        >
          <CompSeverityIcon status={status} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyTestCell.propTypes = {
  status: PropTypes.number.isRequired,
  environment: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
  testName: PropTypes.string.isRequired,
  regressedTests: PropTypes.object,
}
