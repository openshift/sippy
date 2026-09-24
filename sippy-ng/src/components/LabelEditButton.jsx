import { Edit } from '@mui/icons-material'
import { IconButton, Tooltip } from '@mui/material'
import { Link } from 'react-router-dom'
import { SippyCapabilitiesContext } from '../App'
import PropTypes from 'prop-types'
import React from 'react'

export function labelEditorPath(labelId) {
  return `/labels/edit/${encodeURIComponent(labelId)}`
}

// LabelEditButton links full label details to the write-enabled label editor.
export default function LabelEditButton({ labelId }) {
  const capabilities = React.useContext(SippyCapabilitiesContext)
  if (!labelId || !capabilities.includes('write_endpoints')) {
    return null
  }

  return (
    <Tooltip title="Edit label">
      <IconButton
        component={Link}
        to={labelEditorPath(labelId)}
        target="_blank"
        rel="noopener noreferrer"
        size="small"
        aria-label={`Edit label ${labelId}`}
      >
        <Edit fontSize="small" />
      </IconButton>
    </Tooltip>
  )
}

LabelEditButton.propTypes = {
  labelId: PropTypes.string,
}
