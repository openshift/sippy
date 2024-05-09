import { Box, Button, Tooltip, Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'

// This is used as a convenient way to capture the page URL for use with
// the triaging workflow.
export default function CopyPageURL(props) {
  const handleCopyClick = () => {
    navigator.clipboard.writeText(props.apiCallStr)
  }

  return (
    <Box align="left">
      <Typography variant="caption">
        <Tooltip title={props.apiCallStr}>
          <Button onClick={handleCopyClick}>copy page URL</Button>
        </Tooltip>
      </Typography>
    </Box>
  )
}

CopyPageURL.propTypes = {
  apiCallStr: PropTypes.string,
}
