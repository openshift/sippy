import { Button, Tooltip } from '@mui/material'
import { GetApp } from '@mui/icons-material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function GridToolbarDownload(props) {
  let handleClick = () => {
    const dataStr =
      'data:text/json;charset=utf-8,' +
      encodeURIComponent(JSON.stringify(props.getData(), null, 2))
    const a = document.createElement('a')
    a.href = dataStr
    a.download =
      (props.filePrefix === '' ? 'data-' : props.filePrefix + '-') +
      new Date().toISOString() +
      '.json'
    a.click()
  }

  return (
    <Fragment>
      <Tooltip title="Download the raw data">
        <Button
          aria-controls="download-menu"
          aria-haspopup="true"
          startIcon={<GetApp />}
          color="primary"
          onClick={handleClick}
        >
          Download
        </Button>
      </Tooltip>
    </Fragment>
  )
}

GridToolbarDownload.propTypes = {
  filePrefix: PropTypes.string,
  getData: PropTypes.func.isRequired,
}
