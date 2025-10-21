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

export default function SlashCommandModal({
  open,
  onClose,
  prompt,
  onSubmit,
  disabled = false,
}) {
  const classes = useStyles()
  const { renderPrompt } = usePrompts()
  const [formValues, setFormValues] = useState({})
  const [preview, setPreview] = useState('')
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewExpanded, setPreviewExpanded] = useState(false)
  const [autocompleteOptions, setAutocompleteOptions] = useState({})
  const [autocompleteLoading, setAutocompleteLoading] = useState({})

  // Helper function to generate form field helper text
  const getHelperText = (argument) => {
    if (argument.default !== undefined && !argument.required) {
      return `Optional - Defaults to ${JSON.stringify(argument.default)}`
    }
    if (argument.required && argument.type === 'array') {
      return 'Required - Type and press Enter to add values'
    }
    if (argument.type === 'array') {
      return 'Optional - Type and press Enter to add values'
    }
    return argument.required ? 'Required' : 'Optional'
  }

  // Initialize form values with defaults and prefilled args
  useEffect(() => {
    if (!prompt) return

    const initialValues = {}
    prompt.arguments?.forEach((argument) => {
      // Priority: prefilledArgs > default value > null
      if (
        prompt.prefilledArgs &&
        prompt.prefilledArgs[argument.name] !== undefined
      ) {
        initialValues[argument.name] = prompt.prefilledArgs[argument.name]
      } else if (argument.default !== undefined) {
        initialValues[argument.name] = argument.default
      } else {
        initialValues[argument.name] = null
      }
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

    return prompt.arguments.every((argument) => {
      if (!argument.required) return true

      const value = formValues[argument.name]

      // For arrays, check if it has at least one item
      if (argument.type === 'array') {
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
  const renderFormField = (argument) => {
    const hasAutocomplete = argument.autocomplete
    const isArray = argument.type === 'array'

    // Array field with autocomplete
    if (isArray && hasAutocomplete) {
      return (
        <Autocomplete
          key={argument.name}
          multiple
          freeSolo
          options={autocompleteOptions[argument.autocomplete] || []}
          value={formValues[argument.name] || []}
          onChange={(_, newValue) => handleFieldChange(argument.name, newValue)}
          onInputChange={(_, value) => {
            if (value && hasAutocomplete) {
              fetchAutocompleteOptions(argument.autocomplete, value)
            }
          }}
          onOpen={() => {
            if (
              hasAutocomplete &&
              !autocompleteOptions[argument.autocomplete]
            ) {
              fetchAutocompleteOptions(argument.autocomplete)
            }
          }}
          loading={autocompleteLoading[argument.autocomplete]}
          renderInput={(params) => (
            <TextField
              {...params}
              label={argument.description}
              required={argument.required}
              helperText={getHelperText(argument)}
              InputProps={{
                ...params.InputProps,
                endAdornment: (
                  <>
                    {autocompleteLoading[argument.autocomplete] ? (
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
          key={argument.name}
          multiple
          freeSolo
          options={[]}
          value={formValues[argument.name] || []}
          onChange={(_, newValue) => handleFieldChange(argument.name, newValue)}
          renderInput={(params) => (
            <TextField
              {...params}
              label={argument.description}
              required={argument.required}
              helperText={getHelperText(argument)}
            />
          )}
        />
      )
    }

    // Single value field with autocomplete
    if (hasAutocomplete) {
      return (
        <Autocomplete
          key={argument.name}
          freeSolo
          options={autocompleteOptions[argument.autocomplete] || []}
          value={formValues[argument.name] || ''}
          onChange={(_, newValue) => handleFieldChange(argument.name, newValue)}
          onInputChange={(_, value) => {
            handleFieldChange(argument.name, value)
            if (value && hasAutocomplete) {
              fetchAutocompleteOptions(argument.autocomplete, value)
            }
          }}
          onOpen={() => {
            if (
              hasAutocomplete &&
              !autocompleteOptions[argument.autocomplete]
            ) {
              fetchAutocompleteOptions(argument.autocomplete)
            }
          }}
          loading={autocompleteLoading[argument.autocomplete]}
          renderInput={(params) => (
            <TextField
              {...params}
              label={argument.description}
              required={argument.required}
              helperText={getHelperText(argument)}
              InputProps={{
                ...params.InputProps,
                endAdornment: (
                  <>
                    {autocompleteLoading[argument.autocomplete] ? (
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
        key={argument.name}
        label={argument.description}
        value={formValues[argument.name] || ''}
        onChange={(e) => handleFieldChange(argument.name, e.target.value)}
        required={argument.required}
        helperText={getHelperText(argument)}
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
          {prompt.arguments?.map((argument) => renderFormField(argument))}
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
          disabled={!isFormValid() || disabled}
        >
          Use Prompt
        </Button>
      </DialogActions>
    </Dialog>
  )
}

SlashCommandModal.propTypes = {
  open: PropTypes.bool.isRequired,
  disabled: PropTypes.bool,
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
    prefilledArgs: PropTypes.object,
  }),
  onSubmit: PropTypes.func.isRequired,
}
