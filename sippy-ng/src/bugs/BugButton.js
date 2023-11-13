import { Button, IconButton, makeStyles, Modal, TextField } from '@mui/material'
import { styled } from '@mui/material/styles';
import { safeEncodeURIComponent } from '../helpers'
import CloseIcon from '@mui/icons-material/Close'
import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Typography from '@mui/material/Typography'

const PREFIX = 'BugButton';

const classes = {
  alignedButton: `${PREFIX}-alignedButton`
};

// TODO jss-to-styled codemod: The Fragment root was replaced by div. Change the tag if needed.
const Root = styled('div')((
  {
    theme
  }
) => ({
  [`& .${classes.alignedButton}`]: {
    marginTop: 25,
    float: 'left',
  }
}));

export default function BugButton(props) {

  const [open, setOpen] = useState(false)

  const text = `The following test is failing:
  
  ${props.testName}
  
Additional context here:
  
  ${document.location.href}`

  const handleClick = () => {
    const url = `https://issues.redhat.com/secure/CreateIssueDetails!init.jspa?pid=12332330&priority=10200&issuetype=1&components=${
      props.jiraComponentID
    }&description=${safeEncodeURIComponent(text)}`
    window.open(url, '_blank')
  }

  return (
    (<Root>
      <Button
        variant="contained"
        color="primary"
        className={classes.alignedButton}
        onClick={handleClick}
      >
        File a new bug
      </Button>
    </Root>)
  );
}

BugButton.propTypes = {
  jiraComponentID: PropTypes.number,
  testName: PropTypes.string.isRequired,
}
