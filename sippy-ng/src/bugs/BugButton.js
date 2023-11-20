import { Button, IconButton, Modal, TextField } from '@mui/material'
import { makeStyles } from '@mui/styles'
import { safeEncodeURIComponent } from '../helpers'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  alignedButton: {
    marginTop: 25,
    float: 'left',
  },
}))

export default function BugButton(props) {
  const classes = useStyles()
  const [open, setOpen] = useState(false)

  const text = `The following test is failing:
  
  ${props.testName}
  
Additional context here:
  
  ${document.location.href}`

  const handleClick = () => {
    let url = `https://issues.redhat.com/secure/CreateIssueDetails!init.jspa?pid=12332330&priority=10200&issuetype=1&description=${safeEncodeURIComponent(
      text
    )}`

    if (props.jiraComponentID) {
      url += `&components=${props.jiraComponentID}`
    }

    window.open(url, '_blank')
  }

  return (
    <>
      <Button
        variant="contained"
        color="primary"
        className={classes.alignedButton}
        onClick={handleClick}
      >
        File a new bug
      </Button>
    </>
  )
}

BugButton.propTypes = {
  jiraComponentID: PropTypes.number,
  testName: PropTypes.string.isRequired,
}
