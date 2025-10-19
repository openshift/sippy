import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from '@mui/material'
import { ExpandMore as ExpandMoreIcon } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import { safeEncodeURIComponent } from '../helpers'
import { usePrompts } from './store/useChatStore'
import PropTypes from 'prop-types'
import React, { useEffect, useState } from 'react'

const useStyles = makeStyles((theme) => ({
  dialogPaper: {
    minWidth: 600,
    maxWidth: 800,
  },
  description: {
    marginBottom: theme.spacing(2),
    color: theme.palette.text.secondary,
  },
  formFields: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(2),
    marginBottom: theme.spacing(2),
  },
  previewSection: {
    marginTop: theme.spacing(2),
  },
  previewTitle: {
    marginBottom: theme.spacing(1),
    fontWeight: 600,
  },
  previewContent: {
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    maxHeight: 300,
    overflowY: 'auto',
    fontFamily: 'monospace',
    fontSize: '0.875rem',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  },
}))

export default function SlashCommandModal({ open, onClose, prompt, onSubmit }) {
  const classes = useStyles()
  const { renderPrompt } = usePrompts()
  const [formValues, setFormValues] = useState({})
  const [preview, setPreview] = useState('')
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewExpanded, setPreviewExpanded] = useState(false)
  const [autocompleteOptions, setAutocompleteOptions] = useState({})
  const [autocompleteLoading, setAutocompleteLoading] = useState({})

  // Initialize form values with defaults
  useEffect(() => {
    if (!prompt) return

    const initialValues = {}
    prompt.arguments?.forEach((arg) => {
      initialValues[arg.name] = null
    })
    setFormValues(initialValues)
    setPreview('')
    setPreviewExpanded(false)
  }, [prompt])

  // Update preview when form values change and preview is expanded
  useEffect(() => {
    if (!prompt || !previewExpanded) return

    const updatePreview = async () => {
      setPreviewLoading(true)
      try {
        const rendered = await renderPrompt(prompt.name, formValues)
        setPreview(rendered)
      } catch (error) {
        console.error('Error rendering preview:', error)
        setPreview('Error rendering preview')
      } finally {
        setPreviewLoading(false)
      }
    }

    updatePreview()
  }, [formValues, prompt, renderPrompt, previewExpanded])

  const handleFieldChange = (argName, value) => {
    setFormValues((prev) => ({
      ...prev,
      [argName]: value,
    }))
  }

  const handleSubmit = async () => {
    try {
      // Render prompt if we don't have a preview yet
      let finalPrompt = preview
      if (!finalPrompt) {
        finalPrompt = await renderPrompt(prompt.name, formValues)
      }

      onSubmit(finalPrompt)
      onClose()
    } catch (error) {
      console.error('Error rendering prompt on submit:', error)
    }
  }

  const handleClose = () => {
    setFormValues({})
    setPreview('')
    onClose()
  }

  // Check if all required fields are filled
  const isFormValid = () => {
    if (!prompt || !prompt.arguments) return true

    return prompt.arguments.every((arg) => {
      if (!arg.required) return true

      const value = formValues[arg.name]

      // For arrays, check if it has at least one item
      if (arg.type === 'array') {
        return Array.isArray(value) && value.length > 0
      }

      // For strings, check if it's not null/undefined/empty
      return value !== null && value !== undefined && value !== ''
    })
  }

  // Fetch autocomplete options for a field
  const fetchAutocompleteOptions = async (field, searchQuery = '') => {
    setAutocompleteLoading((prev) => ({ ...prev, [field]: true }))

    try {
      const queryParams = []
      if (searchQuery) {
        queryParams.push('search=' + safeEncodeURIComponent(searchQuery))
      }

      const response = await fetch(
        `${
          process.env.REACT_APP_API_URL
        }/api/autocomplete/${field}?${queryParams.join('&')}`
      )

      if (response.ok) {
        const values = await response.json()
        setAutocompleteOptions((prev) => ({
          ...prev,
          [field]: values || [],
        }))
      }
    } catch (error) {
      console.error('Error fetching autocomplete options:', error)
    } finally {
      setAutocompleteLoading((prev) => ({ ...prev, [field]: false }))
    }
  }

  // Render a form field based on argument type and autocomplete setting
  const renderFormField = (arg) => {
    const hasAutocomplete = arg.autocomplete
    const isArray = arg.type === 'array'

    // Array field with autocomplete
    if (isArray && hasAutocomplete) {
      return (
        <Autocomplete
          key={arg.name}
          multiple
          freeSolo
          options={autocompleteOptions[arg.autocomplete] || []}
          value={formValues[arg.name] || []}
          onChange={(_, newValue) => handleFieldChange(arg.name, newValue)}
          onInputChange={(_, value) => {
            if (value && hasAutocomplete) {
              fetchAutocompleteOptions(arg.autocomplete, value)
            }
          }}
          onOpen={() => {
            if (hasAutocomplete && !autocompleteOptions[arg.autocomplete]) {
              fetchAutocompleteOptions(arg.autocomplete)
            }
          }}
          loading={autocompleteLoading[arg.autocomplete]}
          renderInput={(params) => (
            <TextField
              {...params}
              label={arg.description}
              required={arg.required}
              helperText={
                arg.required
                  ? 'Required - Type and press Enter to add values'
                  : 'Optional - Type and press Enter to add values'
              }
              InputProps={{
                ...params.InputProps,
                endAdornment: (
                  <>
                    {autocompleteLoading[arg.autocomplete] ? (
                      <CircularProgress color="inherit" size={20} />
                    ) : null}
                    {params.InputProps.endAdornment}
                  </>
                ),
              }}
            />
          )}
        />
      )
    }

    // Array field without autocomplete
    if (isArray) {
      return (
        <Autocomplete
          key={arg.name}
          multiple
          freeSolo
          options={[]}
          value={formValues[arg.name] || []}
          onChange={(_, newValue) => handleFieldChange(arg.name, newValue)}
          renderInput={(params) => (
            <TextField
              {...params}
              label={arg.description}
              required={arg.required}
              helperText={
                arg.required
                  ? 'Required - Type and press Enter to add values'
                  : 'Optional - Type and press Enter to add values'
              }
            />
          )}
        />
      )
    }

    // Single value field with autocomplete
    if (hasAutocomplete) {
      return (
        <Autocomplete
          key={arg.name}
          freeSolo
          options={autocompleteOptions[arg.autocomplete] || []}
          value={formValues[arg.name] || ''}
          onChange={(_, newValue) => handleFieldChange(arg.name, newValue)}
          onInputChange={(_, value) => {
            handleFieldChange(arg.name, value)
            if (value && hasAutocomplete) {
              fetchAutocompleteOptions(arg.autocomplete, value)
            }
          }}
          onOpen={() => {
            if (hasAutocomplete && !autocompleteOptions[arg.autocomplete]) {
              fetchAutocompleteOptions(arg.autocomplete)
            }
          }}
          loading={autocompleteLoading[arg.autocomplete]}
          renderInput={(params) => (
            <TextField
              {...params}
              label={arg.description}
              required={arg.required}
              helperText={arg.required ? 'Required' : 'Optional'}
              InputProps={{
                ...params.InputProps,
                endAdornment: (
                  <>
                    {autocompleteLoading[arg.autocomplete] ? (
                      <CircularProgress color="inherit" size={20} />
                    ) : null}
                    {params.InputProps.endAdornment}
                  </>
                ),
              }}
            />
          )}
        />
      )
    }

    // Simple text field
    return (
      <TextField
        key={arg.name}
        label={arg.description}
        value={formValues[arg.name] || ''}
        onChange={(e) => handleFieldChange(arg.name, e.target.value)}
        required={arg.required}
        helperText={arg.required ? 'Required' : 'Optional'}
        fullWidth
      />
    )
  }

  if (!prompt) return null

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      classes={{ paper: classes.dialogPaper }}
      maxWidth="md"
      fullWidth
    >
      <DialogTitle>/{prompt.name}</DialogTitle>
      <DialogContent>
        <Typography className={classes.description} variant="body2">
          {prompt.description}
        </Typography>

        <div className={classes.formFields}>
          {prompt.arguments?.map((arg) => renderFormField(arg))}
        </div>

        <Accordion
          expanded={previewExpanded}
          onChange={(_, isExpanded) => setPreviewExpanded(isExpanded)}
          className={classes.previewSection}
        >
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography variant="subtitle2">Preview</Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Box className={classes.previewContent}>
              {previewLoading ? (
                <CircularProgress size={20} />
              ) : (
                preview || 'Preview will appear here'
              )}
            </Box>
          </AccordionDetails>
        </Accordion>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          color="primary"
          disabled={!isFormValid()}
        >
          Use Prompt
        </Button>
      </DialogActions>
    </Dialog>
  )
}

SlashCommandModal.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  prompt: PropTypes.shape({
    name: PropTypes.string.isRequired,
    description: PropTypes.string,
    arguments: PropTypes.arrayOf(
      PropTypes.shape({
        name: PropTypes.string.isRequired,
        description: PropTypes.string,
        required: PropTypes.bool,
        type: PropTypes.string,
        autocomplete: PropTypes.string,
      })
    ),
  }),
  onSubmit: PropTypes.func.isRequired,
}
