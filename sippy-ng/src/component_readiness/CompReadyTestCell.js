import './ComponentReadiness.css'
//import { expandEnvironment } from './CompReadyUtils'
import { CompReadyVarsContext } from '../CompReadyVars'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import TableCell from '@material-ui/core/TableCell'

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
  } = props
  const theme = useTheme()

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

  // Construct a URL with all existing filters plus testId, environment, and testName.
  // This is the url used when you click inside a TableCell on page4 on the right.
  // We pass these arguments to the component that generates the test details report.
  function generateTestReport(
    testId,
    environmentVal,
    filterVals,
    componentName,
    capabilityName,
    testName
  ) {
    const safeComponentName = safeEncodeURIComponent(componentName)
    const safeTestId = safeEncodeURIComponent(testId)
    const safeTestName = safeEncodeURIComponent(testName)
    const retUrl =
      '/component_readiness/test_details' +
      filterVals +
      `&testId=${safeTestId}` +
      expandEnvironment(environmentVal) +
      `&component=${safeComponentName}` +
      `&capability=${capabilityName}` +
      `&testName=${safeTestName}`

    return retUrl
  }

  const handleClick = (event) => {
    if (!event.metaKey) {
      event.preventDefault()
      setComponentParam(component)
      setCapabilityParam(capability)
      setTestIdParam(testId)
      setEnvironmentParam(environment)
      setTestNameParam(testName)
      window.location.href =
        '/sippy-ng' +
        generateTestReport(
          testId,
          environment,
          filterVals,
          component,
          capability,
          testName
        )
    }
  }

  if (status === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          className="cr-cell-result"
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
        className="cr-cell-result"
        style={{
          textAlign: 'center',
          backgroundColor: 'white',
        }}
      >
        <Link
          to={generateTestReport(
            testId,
            environment,
            filterVals,
            component,
            capability,
            testName
          )}
          onClick={handleClick}
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
}
