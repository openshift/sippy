import { Box, ListItemText } from '@mui/material'
import JiraBugLinks from './JiraBugLinks'
import LabelEditButton from './LabelEditButton'
import PropTypes from 'prop-types'
import React from 'react'
import ReactMarkdown from 'react-markdown'

// JobRunLabelDetails renders a label consistently in job-run detail lists.
export default function JobRunLabelDetails({ label, labelId }) {
  return (
    <ListItemText
      primary={
        <Box component="span" display="flex" alignItems="center" gap={0.5}>
          {label ? label.label_title : labelId}
          {label && <LabelEditButton labelId={label.id || labelId} />}
        </Box>
      }
      secondary={
        label ? (
          <Box component="div">
            {label.explanation && (
              <ReactMarkdown>{label.explanation}</ReactMarkdown>
            )}
            {label.bugs?.length > 0 && (
              <JiraBugLinks bugs={label.bugs} showLabel />
            )}
          </Box>
        ) : (
          'Label not found'
        )
      }
    />
  )
}

JobRunLabelDetails.propTypes = {
  label: PropTypes.shape({
    bugs: PropTypes.arrayOf(PropTypes.string),
    explanation: PropTypes.string,
    id: PropTypes.string,
    label_title: PropTypes.string.isRequired,
  }),
  labelId: PropTypes.string.isRequired,
}
