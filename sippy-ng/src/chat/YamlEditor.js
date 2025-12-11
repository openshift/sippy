import {
  atomOneDark,
  atomOneLight,
} from 'react-syntax-highlighter/dist/esm/styles/hljs'
import { Box, TextField } from '@mui/material'
import { makeStyles } from '@mui/styles'
import { Light as SyntaxHighlighter } from 'react-syntax-highlighter'
import { useTheme } from '@mui/material/styles'
import PropTypes from 'prop-types'
import React from 'react'
import yaml from 'react-syntax-highlighter/dist/esm/languages/hljs/yaml'

// Register YAML language
SyntaxHighlighter.registerLanguage('yaml', yaml)

const useStyles = makeStyles((theme) => ({
  editorContainer: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    minHeight: 400,
    overflow: 'hidden',
  },
  textFieldContainer: {
    flex: 1,
    display: 'flex',
    overflow: 'hidden',
    '& .MuiTextField-root': {
      flex: 1,
    },
    '& .MuiInputBase-root': {
      height: '100%',
      alignItems: 'flex-start',
      fontFamily: 'monospace',
      fontSize: '0.875rem',
    },
    '& textarea': {
      height: '100% !important',
      overflow: 'auto !important',
    },
  },
  previewContainer: {
    flex: 1,
    overflow: 'auto',
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(0, 0, 0, 0.3)'
        : 'rgba(0, 0, 0, 0.02)',
  },
  syntaxHighlighter: {
    margin: 0,
    height: '100%',
    '& code': {
      fontFamily: 'monospace',
      fontSize: '0.875rem',
    },
  },
}))

/**
 * YamlEditor - Component for editing YAML with syntax highlighting
 * Can be used in edit mode (with TextField) or preview mode (read-only with highlighting)
 */
export default function YamlEditor({
  value,
  onChange,
  readOnly = false,
  error = null,
  placeholder = 'Enter YAML here...',
}) {
  const classes = useStyles()
  const theme = useTheme()

  const syntaxTheme = theme.palette.mode === 'dark' ? atomOneDark : atomOneLight

  if (readOnly) {
    return (
      <Box className={classes.previewContainer}>
        <SyntaxHighlighter
          language="yaml"
          style={syntaxTheme}
          className={classes.syntaxHighlighter}
          customStyle={{
            margin: 0,
            padding: '16px',
            background: 'transparent',
          }}
        >
          {value || placeholder}
        </SyntaxHighlighter>
      </Box>
    )
  }

  return (
    <Box className={classes.editorContainer}>
      <div className={classes.textFieldContainer}>
        <TextField
          multiline
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          error={!!error}
          helperText={error}
          fullWidth
          variant="outlined"
          InputProps={{
            style: {
              fontFamily: 'Consolas, Monaco, "Courier New", monospace',
            },
          }}
        />
      </div>
    </Box>
  )
}

YamlEditor.propTypes = {
  value: PropTypes.string.isRequired,
  onChange: PropTypes.func,
  readOnly: PropTypes.bool,
  error: PropTypes.string,
  placeholder: PropTypes.string,
}
