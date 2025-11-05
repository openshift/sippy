import {
  Add as AddIcon,
  AutoAwesome as AutoAwesomeIcon,
  Close as CloseIcon,
  Delete as DeleteIcon,
  History as HistoryIcon,
  Save as SaveIcon,
} from '@mui/icons-material'
import {
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  IconButton,
  List,
  ListItem,
  ListItemText,
  MenuItem,
  Select,
  Switch,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material'
import {
  getDefaultPromptTemplate,
  promptToYAML,
  validatePromptYAML,
} from './promptSchema'
import { makeStyles } from '@mui/styles'
import { usePrompts } from './store/useChatStore'
import OneShotChatModal from './OneShotChatModal'
import PropTypes from 'prop-types'
import React, { useEffect, useState } from 'react'
import YamlEditor from './YamlEditor'

const useStyles = makeStyles((theme) => ({
  dialogPaper: {
    minWidth: 800,
    maxWidth: '90vw',
    height: '80vh',
  },
  dialogContent: {
    display: 'flex',
    flexDirection: 'column',
    padding: 0,
    height: '100%',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  tabs: {
    borderBottom: `1px solid ${theme.palette.divider}`,
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
  },
  tabContent: {
    flex: 1,
    overflow: 'auto',
    minHeight: 0,
    padding: theme.spacing(2),
  },
  formFields: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(2),
  },
  argumentsList: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    padding: theme.spacing(1),
  },
  argumentItem: {
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    borderRadius: theme.shape.borderRadius,
    marginBottom: theme.spacing(1),
    padding: theme.spacing(1),
  },
  aiRefinementSection: {
    marginTop: theme.spacing(2),
    padding: theme.spacing(2),
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(138, 43, 226, 0.1)'
        : 'rgba(138, 43, 226, 0.05)',
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.primary.main}`,
  },
  versionHistory: {
    marginTop: theme.spacing(2),
  },
  versionItem: {
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
}))

/**
 * PromptEditor - Dialog for creating and editing custom prompts
 * Features: Dual view (YAML/Form), version history, AI refinement
 */
export default function PromptEditor({
  open,
  onClose,
  promptName = null,
  initialYAML = null,
}) {
  const classes = useStyles()
  const {
    saveLocalPrompt,
    updateLocalPrompt,
    deleteLocalPrompt,
    getLocalPrompt,
    serverPrompts,
  } = usePrompts()

  const [viewMode, setViewMode] = useState(1) // 0 = YAML, 1 = Form
  const [yamlContent, setYamlContent] = useState('')
  const [validationErrors, setValidationErrors] = useState([])
  const [versions, setVersions] = useState([])
  const [aiModalOpen, setAiModalOpen] = useState(false)
  const [aiRefinementPrompt, setAiRefinementPrompt] = useState('')
  const [saveError, setSaveError] = useState(null)

  // Form fields (parsed from YAML)
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    hide: false,
    arguments: [],
    prompt: '',
  })

  // Load existing prompt or use default template
  useEffect(() => {
    if (open) {
      if (promptName) {
        // Editing existing prompt
        const existingPrompt = getLocalPrompt(promptName)
        if (existingPrompt) {
          const { createdAt, updatedAt, source, ...cleanPrompt } =
            existingPrompt
          const yaml = promptToYAML(cleanPrompt)
          setYamlContent(yaml)
          setFormData(cleanPrompt)
          setVersions([{ yaml, timestamp: new Date().toISOString() }])
        }
      } else if (initialYAML) {
        // Creating from AI-generated YAML
        console.log(
          'PromptEditor: Received initialYAML, length:',
          initialYAML.length
        )
        console.log('PromptEditor: initialYAML content:', initialYAML)
        setYamlContent(initialYAML)
        parseYAMLToForm(initialYAML)
        setVersions([
          { yaml: initialYAML, timestamp: new Date().toISOString() },
        ])
      } else {
        // New prompt with default template
        const defaultYAML = getDefaultPromptTemplate()
        setYamlContent(defaultYAML)
        parseYAMLToForm(defaultYAML)
        setVersions([
          { yaml: defaultYAML, timestamp: new Date().toISOString() },
        ])
      }
    } else {
      // Reset on close
      setYamlContent('')
      setFormData({
        name: '',
        description: '',
        hide: false,
        arguments: [],
        prompt: '',
      })
      setVersions([])
      setValidationErrors([])
      setSaveError(null)
      setAiRefinementPrompt('')
    }
  }, [open, promptName, initialYAML, getLocalPrompt])

  // Parse YAML to form data
  const parseYAMLToForm = (yamlStr) => {
    const validation = validatePromptYAML(yamlStr)
    if (validation.valid) {
      setFormData(validation.prompt)
      setValidationErrors([])
    } else {
      setValidationErrors(validation.errors)
    }
  }

  // Sync form to YAML when switching to YAML view
  const syncFormToYAML = () => {
    try {
      const yamlStr = promptToYAML(formData)
      setYamlContent(yamlStr)
      setValidationErrors([])
    } catch (error) {
      setValidationErrors([`Failed to convert form to YAML: ${error.message}`])
    }
  }

  // Handle tab change
  const handleTabChange = (event, newValue) => {
    if (newValue === 1 && viewMode === 0) {
      // Switching from Form to YAML
      syncFormToYAML()
    } else if (newValue === 0 && viewMode === 1) {
      // Switching from YAML to Form
      parseYAMLToForm(yamlContent)
    }
    setViewMode(newValue)
  }

  // Handle YAML change
  const handleYAMLChange = (newYAML) => {
    setYamlContent(newYAML)
    parseYAMLToForm(newYAML)
  }

  // Handle form field changes
  const handleFormFieldChange = (field, value) => {
    setFormData((prev) => ({ ...prev, [field]: value }))
  }

  // Handle argument changes
  const addArgument = () => {
    setFormData((prev) => ({
      ...prev,
      arguments: [
        ...(prev.arguments || []),
        {
          name: '',
          description: '',
          required: false,
          type: 'string',
        },
      ],
    }))
  }

  const updateArgument = (index, field, value) => {
    setFormData((prev) => {
      const newArgs = [...(prev.arguments || [])]
      newArgs[index] = { ...newArgs[index], [field]: value }
      return { ...prev, arguments: newArgs }
    })
  }

  const deleteArgument = (index) => {
    setFormData((prev) => ({
      ...prev,
      arguments: (prev.arguments || []).filter((_, i) => i !== index),
    }))
  }

  // Save prompt
  const handleSave = () => {
    // Ensure we have the latest YAML
    const finalYAML = viewMode === 1 ? promptToYAML(formData) : yamlContent

    // Validate
    const validation = validatePromptYAML(finalYAML)
    if (!validation.valid) {
      setValidationErrors(validation.errors)
      return
    }

    try {
      if (promptName) {
        // Update existing
        updateLocalPrompt(promptName, validation.prompt)
      } else {
        // Save new
        saveLocalPrompt(validation.prompt)
      }
      onClose()
    } catch (error) {
      setSaveError(error.message)
    }
  }

  // Delete prompt
  const handleDelete = () => {
    if (
      window.confirm(
        `Are you sure you want to delete the prompt "${promptName}"?`
      )
    ) {
      try {
        deleteLocalPrompt(promptName)
        onClose()
      } catch (error) {
        setSaveError(error.message)
      }
    }
  }

  // AI refinement
  const handleAIRefinement = () => {
    setAiModalOpen(true)
  }

  const handleAIRefinementResult = (result) => {
    // Extract YAML from the result
    const yamlBlockRegex = /```(?:yaml|yml)?\s*\n([\s\S]*?)```/i
    const match = result.match(yamlBlockRegex)

    if (match) {
      const newYAML = match[1].trim()
      // Add to version history
      setVersions((prev) => [
        { yaml: newYAML, timestamp: new Date().toISOString() },
        ...prev.slice(0, 9), // Keep max 10 versions
      ])
      setYamlContent(newYAML)
      parseYAMLToForm(newYAML)
    } else {
      // If no YAML block found, try to use the whole result
      setVersions((prev) => [
        { yaml: result, timestamp: new Date().toISOString() },
        ...prev.slice(0, 9),
      ])
      setYamlContent(result)
      parseYAMLToForm(result)
    }
    setAiRefinementPrompt('')
  }

  // Revert to a previous version
  const handleRevertToVersion = (versionYAML) => {
    setYamlContent(versionYAML)
    parseYAMLToForm(versionYAML)
  }

  // Build AI refinement prompt
  const buildAIPrompt = () => {
    const currentYAML = viewMode === 1 ? promptToYAML(formData) : yamlContent
    return `Adjust this Sippy prompt YAML according to the following request:

Current YAML:
\`\`\`yaml
${currentYAML}
\`\`\`

Requested changes: ${aiRefinementPrompt}

Please provide the updated YAML in a code block. Maintain the same structure and format.`
  }

  return (
    <>
      <Dialog
        open={open}
        onClose={onClose}
        classes={{ paper: classes.dialogPaper }}
        maxWidth={false}
        fullWidth
      >
        <DialogTitle>
          <Box className={classes.header}>
            <Typography variant="h6">
              {promptName ? `Edit Prompt: ${promptName}` : 'Create New Prompt'}
            </Typography>
            <IconButton onClick={onClose} size="small">
              <CloseIcon />
            </IconButton>
          </Box>
        </DialogTitle>

        <DialogContent className={classes.dialogContent}>
          {saveError && (
            <Alert
              severity="error"
              onClose={() => setSaveError(null)}
              sx={{ mb: 2 }}
            >
              {saveError}
            </Alert>
          )}

          {validationErrors.length > 0 && (
            <Alert severity="error" sx={{ mb: 2 }}>
              <Typography variant="body2" fontWeight="bold">
                Validation Errors:
              </Typography>
              <ul style={{ margin: 0, paddingLeft: 20 }}>
                {validationErrors.map((error, idx) => (
                  <li key={idx}>{error}</li>
                ))}
              </ul>
            </Alert>
          )}

          <Tabs
            value={viewMode}
            onChange={handleTabChange}
            className={classes.tabs}
          >
            <Tab label="Form Editor" />
            <Tab label="YAML Editor" />
          </Tabs>

          <Box className={classes.tabContent}>
            {viewMode === 1 && (
              <YamlEditor
                value={yamlContent}
                onChange={handleYAMLChange}
                error={validationErrors.length > 0 ? 'Invalid YAML' : null}
              />
            )}

            {viewMode === 0 && (
              <Box className={classes.formFields}>
                {/* AI Refinement Section */}
                <Box className={classes.aiRefinementSection}>
                  <TextField
                    placeholder="Describe how you want to adjust this prompt..."
                    value={aiRefinementPrompt}
                    onChange={(e) => setAiRefinementPrompt(e.target.value)}
                    fullWidth
                    multiline
                    rows={2}
                    size="small"
                  />
                  <Button
                    startIcon={<AutoAwesomeIcon />}
                    onClick={handleAIRefinement}
                    disabled={!aiRefinementPrompt.trim()}
                    sx={{ mt: 1 }}
                    variant="outlined"
                    color="primary"
                  >
                    Refine with AI
                  </Button>
                </Box>

                <Divider />

                <TextField
                  label="Prompt Name"
                  value={formData.name || ''}
                  onChange={(e) =>
                    handleFormFieldChange('name', e.target.value)
                  }
                  required
                  helperText="Lowercase letters, numbers, and hyphens only"
                  fullWidth
                />

                <TextField
                  label="Description"
                  value={formData.description || ''}
                  onChange={(e) =>
                    handleFormFieldChange('description', e.target.value)
                  }
                  required
                  multiline
                  rows={2}
                  fullWidth
                />

                <FormControlLabel
                  control={
                    <Switch
                      checked={formData.hide || false}
                      onChange={(e) =>
                        handleFormFieldChange('hide', e.target.checked)
                      }
                    />
                  }
                  label="Hide from slash command list"
                />

                <Divider />

                <Box>
                  <Box
                    display="flex"
                    justifyContent="space-between"
                    alignItems="center"
                    mb={1}
                  >
                    <Typography variant="subtitle2">Arguments</Typography>
                    <Button
                      startIcon={<AddIcon />}
                      onClick={addArgument}
                      size="small"
                    >
                      Add Argument
                    </Button>
                  </Box>

                  <Box className={classes.argumentsList}>
                    {(formData.arguments || []).length === 0 ? (
                      <Typography variant="body2" color="textSecondary">
                        No arguments defined. Click &quot;Add Argument&quot; to
                        create one.
                      </Typography>
                    ) : (
                      (formData.arguments || []).map((arg, index) => (
                        <Box key={index} className={classes.argumentItem}>
                          <Box
                            display="flex"
                            justifyContent="space-between"
                            alignItems="center"
                            mb={1}
                          >
                            <Typography variant="caption" fontWeight="bold">
                              Argument {index + 1}
                            </Typography>
                            <IconButton
                              size="small"
                              onClick={() => deleteArgument(index)}
                            >
                              <DeleteIcon fontSize="small" />
                            </IconButton>
                          </Box>

                          <Box display="flex" gap={1} flexDirection="column">
                            <TextField
                              label="Name"
                              value={arg.name || ''}
                              onChange={(e) =>
                                updateArgument(index, 'name', e.target.value)
                              }
                              size="small"
                              fullWidth
                            />
                            <TextField
                              label="Description"
                              value={arg.description || ''}
                              onChange={(e) =>
                                updateArgument(
                                  index,
                                  'description',
                                  e.target.value
                                )
                              }
                              size="small"
                              fullWidth
                            />
                            <Box display="flex" gap={1}>
                              <Select
                                value={arg.type || 'string'}
                                onChange={(e) =>
                                  updateArgument(index, 'type', e.target.value)
                                }
                                size="small"
                                sx={{ flex: 1 }}
                              >
                                <MenuItem value="string">String</MenuItem>
                                <MenuItem value="array">Array</MenuItem>
                              </Select>
                              <FormControlLabel
                                control={
                                  <Switch
                                    checked={arg.required || false}
                                    onChange={(e) =>
                                      updateArgument(
                                        index,
                                        'required',
                                        e.target.checked
                                      )
                                    }
                                    size="small"
                                  />
                                }
                                label="Required"
                              />
                            </Box>
                            <TextField
                              label="Autocomplete (optional)"
                              value={arg.autocomplete || ''}
                              onChange={(e) =>
                                updateArgument(
                                  index,
                                  'autocomplete',
                                  e.target.value
                                )
                              }
                              size="small"
                              fullWidth
                              helperText="API autocomplete field name"
                            />
                          </Box>
                        </Box>
                      ))
                    )}
                  </Box>
                </Box>

                <Divider />

                <TextField
                  label="Prompt Template"
                  value={formData.prompt || ''}
                  onChange={(e) =>
                    handleFormFieldChange('prompt', e.target.value)
                  }
                  required
                  multiline
                  rows={32}
                  fullWidth
                  helperText="Use {{ argument_name }} for variable substitution"
                />
              </Box>
            )}

            {/* Version History */}
            {versions.length > 1 && (
              <Box className={classes.versionHistory}>
                <Box display="flex" alignItems="center" gap={1} mb={1}>
                  <HistoryIcon fontSize="small" />
                  <Typography variant="subtitle2">Version History</Typography>
                </Box>
                <List dense>
                  {versions.map((version, index) => (
                    <ListItem
                      key={index}
                      className={classes.versionItem}
                      onClick={() => handleRevertToVersion(version.yaml)}
                    >
                      <ListItemText
                        primary={`Version ${versions.length - index}`}
                        secondary={new Date(version.timestamp).toLocaleString()}
                      />
                      {index === 0 && <Chip label="Current" size="small" />}
                    </ListItem>
                  ))}
                </List>
              </Box>
            )}
          </Box>
        </DialogContent>

        <DialogActions>
          {promptName && (
            <Button
              onClick={handleDelete}
              color="error"
              startIcon={<DeleteIcon />}
              sx={{ mr: 'auto' }}
            >
              Delete
            </Button>
          )}
          <Button onClick={onClose}>Cancel</Button>
          <Button
            onClick={handleSave}
            variant="contained"
            startIcon={<SaveIcon />}
            disabled={validationErrors.length > 0}
          >
            Save
          </Button>
        </DialogActions>
      </Dialog>

      {/* AI Refinement Modal */}
      <OneShotChatModal
        open={aiModalOpen}
        onClose={() => setAiModalOpen(false)}
        prompt={buildAIPrompt()}
        onResult={handleAIRefinementResult}
        title="Refining Prompt with AI"
      />
    </>
  )
}

PromptEditor.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  promptName: PropTypes.string,
  initialYAML: PropTypes.string,
}
