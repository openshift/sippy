import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  IconButton,
  InputLabel,
  Link,
  MenuItem,
  Select,
  Snackbar,
  TextField,
  Typography,
} from '@mui/material'
import { AutoAwesome as AutoAwesomeIcon, Close } from '@mui/icons-material'
import { CapabilitiesContext } from '../App'
import {
  getBugsAPIUrl,
  getTriagesAPIUrl,
} from '../component_readiness/CompReadyUtils'
import { makeStyles } from '@mui/styles'
import { usePrompts } from '../chat/store/useChatStore'
import BugButton from './BugButton'
import OneShotChatModal from '../chat/OneShotChatModal'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useState } from 'react'

const useStyles = makeStyles((theme) => ({
  alignedButton: {
    float: 'left',
  },
  fieldLabel: {
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(1),
  },
  chipContainer: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: theme.spacing(1),
    marginTop: theme.spacing(1),
  },
}))

export default function FileBug({
  testName,
  regressionId,
  component,
  capability,
  context,
  labels = [],
  jiraComponentID,
  jiraComponentName,
  version,
  setHasBeenTriaged,
}) {
  const classes = useStyles()
  const { renderPrompt } = usePrompts()
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [formData, setFormData] = useState({
    summary: '',
    description: '',
    affectsVersions: [],
    components: [],
    labels: [],
    createTriage: true,
    triageType: 'type',
  })
  const [componentInput, setComponentInput] = useState('')
  const [labelInput, setLabelInput] = useState('')
  const [versionInput, setVersionInput] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [errorAlert, setErrorAlert] = useState('')
  const [successAlert, setSuccessAlert] = useState(null)
  const [isValidationError, setIsValidationError] = useState(false)
  const [isAIModalOpen, setIsAIModalOpen] = useState(false)
  const [aiGeneratedDescription, setAiGeneratedDescription] = useState('')
  const [aiPrompt, setAiPrompt] = useState('')
  const [promptRenderError, setPromptRenderError] = useState(null)
  const capabilities = useContext(CapabilitiesContext)
  const chatEnabled = capabilities.includes('chat')

  const handleOpenModal = () => {
    const defaultText = `
The following test is failing more than expected:
  
{code:none}${testName}{code}
  
See the [sippy test details|${document.location.href}] for additional context.
    `

    const defaultSummary =
      component && capability
        ? `Component Readiness: [${component}] [${capability}] test regressed`
        : ''

    // Use AI-generated description if available, otherwise use context or default
    const defaultDescription = aiGeneratedDescription || context || defaultText

    const defaultAffectsVersions = []
    if (version) defaultAffectsVersions.push(version)

    setFormData({
      summary: defaultSummary,
      description: defaultDescription,
      affectsVersions: defaultAffectsVersions,
      components: [],
      componentId: jiraComponentID ? jiraComponentID : '', // This could be NaN, and we don't want to send one in that case
      labels: labels,
      createTriage: regressionId > 0, // Only allow for triage creation if we have a regressionId
      triageType: 'type',
    })

    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setErrorAlert('')
    setSuccessAlert(null)
    setIsSubmitting(false)
    setIsValidationError(false)
  }

  const handleFormChange = (event) => {
    const { name, value, type, checked } = event.target
    setFormData((prev) => ({
      ...prev,
      [name]: type === 'checkbox' ? checked : value,
    }))
  }

  const addChip = (type, value) => {
    if (value.trim() && !formData[type].includes(value.trim())) {
      setFormData((prev) => ({
        ...prev,
        [type]: [...prev[type], value.trim()],
      }))
    }
  }

  const removeChip = (type, chipToRemove) => {
    setFormData((prev) => ({
      ...prev,
      [type]: prev[type].filter((item) => item !== chipToRemove),
    }))
  }

  const handleKeyDown = (type, event, inputValue, inputSetter) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      addChip(type, inputValue)
      inputSetter('')
    }
  }

  const handleSubmit = () => {
    setIsSubmitting(true)
    setErrorAlert('')
    setSuccessAlert(null)
    setIsValidationError(false)

    const validationErrors = []
    if (!formData.summary.trim()) {
      validationErrors.push('Summary is required')
    }
    if (!formData.description.trim()) {
      validationErrors.push('Description is required')
    }
    if (!jiraComponentID && formData.components.length === 0) {
      validationErrors.push('At least one component is required')
    }
    if (formData.affectsVersions.length === 0) {
      validationErrors.push('At least one affects version(s) is required')
    }
    if (formData.createTriage && formData.triageType === 'type') {
      validationErrors.push(
        'Triage type is required when electing to create a Triage entry'
      )
    }

    if (validationErrors.length > 0) {
      setErrorAlert(validationErrors.join('; '))
      setIsValidationError(true)
      setIsSubmitting(false)
      return
    }

    const bugData = {
      summary: formData.summary,
      description: formData.description,
      components: formData.components,
      component_id: String(formData.componentId),
      labels: formData.labels,
      affects_versions: formData.affectsVersions,
    }

    fetch(getBugsAPIUrl(), {
      method: 'POST',
      body: JSON.stringify(bugData),
    })
      .then((response) => response.json())
      .then((bugResult) => {
        if (!bugResult.success) {
          throw new Error('Failed to create Jira issue')
        }

        if (formData.createTriage) {
          const triageData = {
            description: formData.summary,
            url: bugResult.jira_url,
            regressions: [{ id: regressionId }],
            type: formData.triageType,
          }

          return fetch(getTriagesAPIUrl(), {
            method: 'POST',
            body: JSON.stringify(triageData),
          }).then((triageResponse) => {
            if (!triageResponse.ok) {
              return triageResponse.json().then((data) => {
                let errorMessage =
                  'error creating Triage entry: invalid response returned from server'
                if (data?.code) {
                  errorMessage =
                    'error creating Triage entry: ' +
                    data.code +
                    ': ' +
                    data.message
                }
                return { bugResult, triageMessage: errorMessage }
              })
            }
            return {
              bugResult,
              triageMessage: 'Matching Triage entry successfully created',
              triageCreated: true,
            }
          })
        } else {
          return { bugResult, triageMessage: '' }
        }
      })
      .then(({ bugResult, triageMessage, triageCreated }) => {
        const successMessage = {
          text:
            'Successfully created Jira Issue: ' +
            (bugResult.dry_run ? '(dry-run only, no issue created): ' : ''),
          link: bugResult.jira_url,
          linkText: bugResult.jira_key,
          triageMessage: triageCreated ? triageMessage : '',
        }
        setSuccessAlert(successMessage)

        if (formData.createTriage && !triageCreated) {
          setErrorAlert(triageMessage)
        }

        setIsModalOpen(false)

        if (triageCreated) {
          // if we created a triage entry, refresh the page after a slight timeout to allow the message to be read
          setTimeout(() => {
            setHasBeenTriaged(true)
          }, 3000)
        }
      })
      .catch((error) => {
        console.error('Error filing bug or triage:', error)
        setErrorAlert(`Failed to create Jira issue: ${error.message}`)
        setIsValidationError(false)
      })
      .finally(() => {
        setIsSubmitting(false)
      })
  }

  const clearAlerts = () => {
    setSuccessAlert(null)
    setErrorAlert('')
    setIsValidationError(false)
  }

  const triageTypeOptions = [
    'type',
    'ci-infra',
    'product-infra',
    'product',
    'test',
  ]

  const handleGenerateAIDescription = async () => {
    setPromptRenderError(null)
    try {
      // Render the prompt with arguments
      const rendered = await renderPrompt(
        'component-readiness-jira-description',
        {
          test_name: testName,
          url: window.location.href,
        }
      )
      setAiPrompt(rendered)
      setIsAIModalOpen(true)
    } catch (error) {
      console.error('Failed to render prompt:', error)
      setPromptRenderError(
        `Failed to load AI prompt: ${error.message || error}`
      )
    }
  }

  const handleAIDescriptionResult = (generatedDescription) => {
    const currentUrl = window.location.href
    const descriptionWithNote = `{panel:title=⚠️ AI-Generated Content|borderStyle=dashed|borderColor=#9C27B0|titleBGColor=#F3E5F5}
Sippy AI-assisted description; please review details for accuracy.
{panel}

*Filed from:* [Test Regression Details|${currentUrl}]

${generatedDescription}`
    setAiGeneratedDescription(descriptionWithNote)
    setFormData((prev) => ({
      ...prev,
      description: descriptionWithNote,
    }))
    setIsAIModalOpen(false)
  }

  return (
    <Fragment>
      <Snackbar
        open={errorAlert !== ''}
        onClose={() => {
          setErrorAlert('')
          setIsValidationError(false)
        }}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
        autoHideDuration={null}
        disableWindowBlurListener
      >
        <Alert
          severity="error"
          onClose={() => {
            setErrorAlert('')
            setIsValidationError(false)
          }}
        >
          <Box>
            <Typography variant="body2" sx={{ mb: 1 }}>
              {errorAlert}
            </Typography>
            {!isValidationError && (
              <Box sx={{ mt: 1 }}>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  As an alternative, you can file the bug directly:
                </Typography>
                <BugButton
                  testName={testName}
                  component={component}
                  capability={capability}
                  jiraComponentID={String(jiraComponentID)}
                  labels={formData.labels}
                  context={formData.description}
                />
              </Box>
            )}
          </Box>
        </Alert>
      </Snackbar>
      <Snackbar
        open={successAlert !== null}
        onClose={() => setSuccessAlert(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
        autoHideDuration={6000}
        disableWindowBlurListener
      >
        <Alert severity="success" onClose={() => setSuccessAlert(null)}>
          {successAlert && (
            <Box>
              <Typography variant="body2" component="span">
                {successAlert.text}
                <Link
                  href={successAlert.link}
                  target="_blank"
                  rel="noopener noreferrer"
                  sx={{ color: 'inherit', fontWeight: 'bold' }}
                >
                  {successAlert.linkText}
                </Link>
              </Typography>
              {successAlert.triageMessage && (
                <Typography variant="body2" sx={{ mt: 1 }}>
                  {successAlert.triageMessage}
                </Typography>
              )}
            </Box>
          )}
        </Alert>
      </Snackbar>
      {!isModalOpen && (
        <Button
          variant="contained"
          color="primary"
          className={classes.alignedButton}
          onClick={handleOpenModal}
        >
          File a new bug
        </Button>
      )}

      <Dialog
        open={isModalOpen}
        onClose={handleCloseModal}
        maxWidth={false}
        fullWidth
        PaperProps={{
          style: {
            width: '50vw',
            maxWidth: 'none',
          },
        }}
      >
        <DialogTitle>
          File Jira Bug
          <IconButton
            aria-label="close"
            onClick={handleCloseModal}
            sx={{ position: 'absolute', right: 8, top: 8 }}
          >
            <Close />
          </IconButton>
        </DialogTitle>

        <DialogContent dividers>
          <Typography variant="subtitle2" className={classes.fieldLabel}>
            Summary *
          </Typography>
          <TextField
            name="summary"
            value={formData.summary}
            onChange={handleFormChange}
            fullWidth
            required
            margin="normal"
            helperText="Title of the issue"
          />

          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            <Typography variant="subtitle2" className={classes.fieldLabel}>
              Description *
            </Typography>
            {chatEnabled && (
              <Button
                size="small"
                startIcon={<AutoAwesomeIcon />}
                onClick={handleGenerateAIDescription}
                sx={{
                  background:
                    'linear-gradient(45deg, #9C27B0 30%, #E91E63 90%)',
                  boxShadow: '0 3px 5px 2px rgba(156, 39, 176, .3)',
                  color: 'white',
                  fontWeight: 'bold',
                  textTransform: 'none',
                  transition: 'all 0.3s ease',
                  animation: 'pulse 2s ease-in-out infinite',
                  '@keyframes pulse': {
                    '0%, 100%': {
                      boxShadow: '0 3px 5px 2px rgba(156, 39, 176, .3)',
                    },
                    '50%': {
                      boxShadow: '0 3px 15px 5px rgba(156, 39, 176, .5)',
                    },
                  },
                  '&:hover': {
                    background:
                      'linear-gradient(45deg, #7B1FA2 30%, #C2185B 90%)',
                    boxShadow: '0 6px 20px 4px rgba(156, 39, 176, .4)',
                    transform: 'translateY(-2px)',
                  },
                }}
              >
                Generate AI-enhanced Description
              </Button>
            )}
          </Box>
          <TextField
            name="description"
            value={formData.description}
            onChange={handleFormChange}
            fullWidth
            required
            multiline
            minRows={5}
            margin="normal"
            helperText="Detailed description including test failure information"
          />

          <Typography variant="subtitle2" className={classes.fieldLabel}>
            Affects Versions *
          </Typography>
          <TextField
            placeholder="Add versions (press Enter to add)"
            value={versionInput}
            onChange={(e) => setVersionInput(e.target.value)}
            onKeyDown={(e) =>
              handleKeyDown('affectsVersions', e, versionInput, setVersionInput)
            }
            fullWidth
            margin="normal"
          />
          <Box className={classes.chipContainer}>
            {formData.affectsVersions.map((version) => (
              <Chip
                key={version}
                label={version}
                onDelete={() => removeChip('affectsVersions', version)}
                color="info"
                variant="outlined"
              />
            ))}
          </Box>

          <Typography variant="subtitle2" className={classes.fieldLabel}>
            Components *
          </Typography>
          <TextField
            placeholder="Add components (press Enter to add)"
            value={componentInput}
            onChange={(e) => setComponentInput(e.target.value)}
            onKeyDown={(e) =>
              handleKeyDown('components', e, componentInput, setComponentInput)
            }
            fullWidth
            margin="normal"
            helperText={
              isNaN(jiraComponentID)
                ? 'Could not determine Jira component ID, a component must be provided'
                : ''
            }
          />
          <Box className={classes.chipContainer}>
            {(!isNaN(jiraComponentID) || jiraComponentName) && (
              <Chip
                label={`Computed Component: ${jiraComponentName || 'Unknown'}${
                  !isNaN(jiraComponentID) ? ` (ID: ${jiraComponentID})` : ''
                }`}
                color="primary"
                variant="outlined"
              />
            )}
            {formData.components.map((component) => (
              <Chip
                key={component}
                label={component}
                onDelete={() => removeChip('components', component)}
                color="primary"
                variant="outlined"
              />
            ))}
          </Box>

          <Typography variant="subtitle2" className={classes.fieldLabel}>
            Labels
          </Typography>
          <TextField
            placeholder="Add labels (press Enter to add)"
            value={labelInput}
            onChange={(e) => setLabelInput(e.target.value)}
            onKeyDown={(e) =>
              handleKeyDown('labels', e, labelInput, setLabelInput)
            }
            fullWidth
            margin="normal"
          />
          <Box className={classes.chipContainer}>
            {formData.labels.map((label) => (
              <Chip
                key={label}
                label={label}
                onDelete={() => removeChip('labels', label)}
                color="secondary"
                variant="outlined"
              />
            ))}
          </Box>

          <FormControlLabel
            control={
              <Checkbox
                name="createTriage"
                color="primary"
                checked={formData.createTriage}
                onChange={handleFormChange}
                disabled={regressionId <= 0}
              />
            }
            label="Create triage record"
            sx={{ mt: 2, mb: 1 }}
          />

          {formData.createTriage && (
            <FormControl fullWidth margin="normal">
              <InputLabel>Triage Type</InputLabel>
              <Select
                name="triageType"
                label="Triage Type"
                value={formData.triageType}
                onChange={handleFormChange}
              >
                {triageTypeOptions.map((option, index) => (
                  <MenuItem key={index} value={option}>
                    {option}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          )}
        </DialogContent>

        <DialogActions
          sx={{ justifyContent: 'space-between', alignItems: 'center' }}
        >
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button
              variant="contained"
              color="secondary"
              onClick={handleCloseModal}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              color="primary"
              onClick={handleSubmit}
              disabled={
                isSubmitting ||
                !formData.summary.trim() ||
                !formData.description.trim()
              }
            >
              {isSubmitting ? 'Filing Bug...' : 'File Bug'}
            </Button>
          </Box>
          <Typography
            variant="body2"
            sx={{
              fontStyle: 'italic',
              color: 'text.secondary',
              textAlign: 'right',
            }}
          >
            Note: All fields can be edited after the bug is submitted to Jira.
          </Typography>
        </DialogActions>
      </Dialog>

      {chatEnabled && (
        <OneShotChatModal
          open={isAIModalOpen}
          onClose={() => setIsAIModalOpen(false)}
          prompt={aiPrompt}
          onResult={handleAIDescriptionResult}
          title="Generating AI-Enhanced Bug Description"
        />
      )}

      {/* Error snackbar for prompt rendering failures */}
      <Snackbar
        open={!!promptRenderError}
        autoHideDuration={6000}
        onClose={() => setPromptRenderError(null)}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        <Alert severity="error" onClose={() => setPromptRenderError(null)}>
          {promptRenderError}
        </Alert>
      </Snackbar>
    </Fragment>
  )
}

FileBug.propTypes = {
  testName: PropTypes.string.isRequired,
  regressionId: PropTypes.number.isRequired,
  component: PropTypes.string,
  capability: PropTypes.string,
  context: PropTypes.string,
  labels: PropTypes.array,
  jiraComponentID: PropTypes.number,
  jiraComponentName: PropTypes.string,
  version: PropTypes.string,
  setHasBeenTriaged: PropTypes.func.isRequired,
}
