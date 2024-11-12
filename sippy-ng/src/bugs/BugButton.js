import { Button, IconButton, Modal, TextField } from '@mui/material'
import { makeStyles } from '@mui/styles'
import { safeEncodeURIComponent } from '../helpers'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  alignedButton: {
    float: 'left',
  },
}))

export default function BugButton(props) {
  const classes = useStyles()
  const [open, setOpen] = useState(false)

  const text = `
The following test is failing more than expected:
  
{code:none}${props.testName}{code}
  
See the [sippy test details|${document.location.href}] for additional context.
  `

  const summary = `Component Readiness: [${props.component}] [${props.capability}] test regressed`

  const handleClick = () => {
    let message = props.context || text
    let url = `https://issues.redhat.com/secure/CreateIssueDetails!init.jspa?pid=12332330&priority=10200&issuetype=1&description=${safeEncodeURIComponent(
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
      className={classes.alignedButton}
      onClick={handleClick}
    >
      File a new bug
    </Button>
  )
}

BugButton.propTypes = {
  jiraComponentID: PropTypes.string,
  component: PropTypes.string,
  capability: PropTypes.string,
  context: PropTypes.string,
  labels: PropTypes.array,
  testName: PropTypes.string.isRequired,
}
