import { Edit as EditIcon, Preview } from '@mui/icons-material'
import {
  Paper,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

export default function MarkdownEditor({
  value,
  onChange,
  label,
  helperText,
  disabled = false,
  minRows,
  rows = 4,
  size = 'small',
}) {
  const [viewMode, setViewMode] = React.useState('edit')
  const labelId = React.useId()
  const fixedRows = minRows === undefined ? rows : undefined

  return (
    <Stack spacing={1}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography id={labelId} variant="caption" color="text.secondary">
          {label}
        </Typography>
        <ToggleButtonGroup
          value={viewMode}
          exclusive
          onChange={(event, newMode) => {
            if (newMode !== null) setViewMode(newMode)
          }}
          size="small"
          disabled={disabled}
          aria-label={`${label} mode`}
        >
          <ToggleButton value="edit" aria-label="Edit">
            <EditIcon fontSize="small" />
            <Typography variant="caption" sx={{ ml: 0.5 }}>
              Edit
            </Typography>
          </ToggleButton>
          <ToggleButton value="preview" aria-label="Preview">
            <Preview fontSize="small" />
            <Typography variant="caption" sx={{ ml: 0.5 }}>
              Preview
            </Typography>
          </ToggleButton>
        </ToggleButtonGroup>
      </Stack>

      {viewMode === 'edit' ? (
        <TextField
          fullWidth
          multiline
          rows={fixedRows}
          minRows={minRows}
          value={value}
          onChange={onChange}
          helperText={helperText}
          placeholder="Enter markdown text..."
          size={size}
          disabled={disabled}
          inputProps={{ 'aria-labelledby': labelId }}
        />
      ) : (
        <Paper
          variant="outlined"
          role="region"
          aria-label={`${label} preview`}
          sx={{
            p: 2,
            minHeight: '120px',
            backgroundColor: (theme) =>
              theme.palette.mode === 'dark'
                ? 'rgba(255, 255, 255, 0.05)'
                : 'grey.50',
          }}
        >
          {value ? (
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{value}</ReactMarkdown>
          ) : (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ fontStyle: 'italic' }}
            >
              No content to preview
            </Typography>
          )}
        </Paper>
      )}
    </Stack>
  )
}

MarkdownEditor.propTypes = {
  value: PropTypes.string.isRequired,
  onChange: PropTypes.func.isRequired,
  label: PropTypes.string.isRequired,
  helperText: PropTypes.string,
  disabled: PropTypes.bool,
  minRows: PropTypes.number,
  rows: PropTypes.number,
  size: PropTypes.oneOf(['medium', 'small']),
}
