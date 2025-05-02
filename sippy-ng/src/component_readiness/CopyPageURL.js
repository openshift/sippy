import { Box, Tooltip, Typography } from '@mui/material'
import Link from '@mui/material/Link'
import PropTypes from 'prop-types'
import React from 'react'

// This is used as a convenient way to capture the page URL for use with
// the triaging workflow.
export default function CopyPageURL(props) {
  return (
    <Box align="right">
      <Typography variant="caption">
        <Tooltip title={props.apiCallStr}>
          <Link href={props.apiCallStr} target="_blank">
            API URL
          </Link>
        </Tooltip>
      </Typography>
    </Box>
  )
}

CopyPageURL.propTypes = {
  apiCallStr: PropTypes.string,
}
