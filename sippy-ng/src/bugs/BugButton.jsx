import { Button } from '@mui/material'
import { safeEncodeURIComponent } from '../helpers'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

export default function BugButton({ jiraProjectID = '10325', ...props }) {
  const [_open, _setOpen] = useState(false)

  const text = `
The following test is failing more than expected:
  
{code:none}${props.testName}{code}
  
See the [sippy test details|${document.location.href}] for additional context.
  `

  const summary = `[${props.component}] [${props.capability}] test regressed`

  const handleClick = () => {
    let message = props.context || text
    let url = `https://redhat.atlassian.net/secure/CreateIssueDetails!init.jspa?pid=${jiraProjectID}&issuetype=10016&description=${safeEncodeURIComponent(
      message
    )}`

    if (props.jiraComponentID) {
      url += `&components=${props.jiraComponentID}`
    }
    if (props.component) {
      url += `&summary=${safeEncodeURIComponent(summary)}`
    }

    if (Array.isArray(props.labels) && props.labels.length > 0) {
      props.labels.forEach((label) => {
        url += `&labels=${safeEncodeURIComponent(label)}`
      })
    }

    window.open(url, '_blank')
  }

  return (
    <Button
      variant="contained"
      color="primary"
      sx={{ float: 'left' }}
      onClick={handleClick}
    >
      File a new bug
    </Button>
  )
}

BugButton.propTypes = {
  jiraProjectID: PropTypes.string,
  jiraComponentID: PropTypes.string,
  component: PropTypes.string,
  capability: PropTypes.string,
  context: PropTypes.string,
  labels: PropTypes.array,
  testName: PropTypes.string.isRequired,
}
