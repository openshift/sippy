import { Alert } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'

export default function WarningsBanner({ warnings }) {
  if (!warnings || warnings.length === 0) {
    return null
  }

  return (
    <Alert severity="warning" style={{ marginBottom: '16px' }}>
      <strong>Warning:</strong>
      <ul style={{ marginTop: '8px', marginBottom: '0' }}>
        {warnings.map((warning, idx) => (
          <li key={idx}>{warning}</li>
        ))}
      </ul>
    </Alert>
  )
}

WarningsBanner.propTypes = {
  warnings: PropTypes.arrayOf(PropTypes.string),
}
