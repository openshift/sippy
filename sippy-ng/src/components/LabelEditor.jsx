import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import { Delete } from '@mui/icons-material'
import { isValidJiraKey, normalizeJiraKeys } from './JiraBugLinks'
import { labelEditorPath } from './LabelEditButton'
import { SippyCapabilitiesContext } from '../App'
import { useNavigate, useParams } from 'react-router-dom'
import MarkdownEditor from './MarkdownEditor'
import React from 'react'

const DISPLAY_CONTEXTS = [
  { value: 'spyglass', label: 'Spyglass' },
  { value: 'metrics', label: 'Metrics' },
  { value: 'jaq-options', label: 'JAQ options' },
]
const DELETE_SUCCESS_DURATION_MS = 10000

function editableLabel(label) {
  return {
    id: label.id,
    label_title: label.label_title || '',
    explanation: label.explanation || '',
    bugs: [...(label.bugs || [])],
    hide_display_contexts: [...(label.hide_display_contexts || [])],
  }
}

function sortLabels(labels) {
  return [...labels].sort(
    (a, b) =>
      (a.label_title || '').localeCompare(b.label_title || '') ||
      a.id.localeCompare(b.id)
  )
}

function labelURL(label) {
  return (
    label.links?.self ||
    `${import.meta.env.VITE_API_URL}/api/jobs/labels/${encodeURIComponent(
      label.id
    )}`
  )
}

async function responseError(response, action) {
  const body = await response.text()
  let detail = body
  try {
    const parsed = JSON.parse(body)
    detail = parsed.message || parsed.detail || body
  } catch {
    // Keep the plain response body when the server did not return JSON.
  }
  return new Error(
    detail
      ? `Failed to ${action} label: ${detail}`
      : `Failed to ${action} label: HTTP ${response.status}`
  )
}

export default function LabelEditor() {
  const capabilities = React.useContext(SippyCapabilitiesContext)
  const writeEndpointsEnabled = capabilities.includes('write_endpoints')
  const { labelId } = useParams()
  const navigate = useNavigate()
  const deleteTimer = React.useRef(null)

  const [labels, setLabels] = React.useState([])
  const [draft, setDraft] = React.useState(null)
  const [loading, setLoading] = React.useState(true)
  const [saving, setSaving] = React.useState(false)
  const [deleting, setDeleting] = React.useState(false)
  const [deletePending, setDeletePending] = React.useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = React.useState(false)
  const [errorMessage, setErrorMessage] = React.useState('')
  const [successMessage, setSuccessMessage] = React.useState('')

  const loadLabels = React.useCallback(async () => {
    const response = await fetch(
      import.meta.env.VITE_API_URL + '/api/jobs/labels'
    )
    if (!response.ok) {
      throw await responseError(response, 'load')
    }
    return sortLabels(await response.json())
  }, [])

  React.useEffect(() => {
    return () => {
      if (deleteTimer.current !== null) {
        clearTimeout(deleteTimer.current)
      }
    }
  }, [])

  React.useEffect(() => {
    if (!writeEndpointsEnabled) {
      setLoading(false)
      return
    }

    let cancelled = false
    setLoading(true)
    setErrorMessage('')
    loadLabels()
      .then((loadedLabels) => {
        if (cancelled) return
        setLabels(loadedLabels)
        if (!labelId && loadedLabels.length > 0) {
          navigate(labelEditorPath(loadedLabels[0].id), { replace: true })
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setErrorMessage(error.message)
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false)
        }
      })

    return () => {
      cancelled = true
    }
  }, [writeEndpointsEnabled, loadLabels])

  React.useEffect(() => {
    const selected = labels.find((label) => label.id === labelId)
    setDraft(selected ? editableLabel(selected) : null)
  }, [labelId, labels])

  if (!writeEndpointsEnabled) {
    return (
      <Container maxWidth="md" sx={{ py: 4 }}>
        <Alert severity="info">
          Label editing is unavailable because writable endpoints are disabled.
        </Alert>
      </Container>
    )
  }

  const selectedLabel = labels.find((label) => label.id === draft?.id)
  const normalizedBugs = normalizeJiraKeys(draft?.bugs)
  const invalidBugs = normalizedBugs.filter((bug) => !isValidJiraKey(bug))
  const duplicateTitle = labels.some(
    (label) =>
      label.id !== draft?.id &&
      label.label_title.toLowerCase() ===
        draft?.label_title.trim().toLowerCase()
  )
  const validDraft =
    draft?.label_title.trim().length > 0 &&
    !duplicateTitle &&
    invalidBugs.length === 0

  const updateDraft = (field, value) => {
    setDraft((current) => ({ ...current, [field]: value }))
    setErrorMessage('')
    setSuccessMessage('')
  }

  const selectLabel = (id) => {
    setErrorMessage('')
    setSuccessMessage('')
    navigate(labelEditorPath(id), { replace: true })
  }

  const saveLabel = async () => {
    if (!validDraft || !selectedLabel) return

    const payload = {
      id: draft.id,
      label_title: draft.label_title.trim(),
      explanation: draft.explanation,
      bugs: normalizedBugs,
      hide_display_contexts: [...draft.hide_display_contexts],
    }

    setSaving(true)
    setErrorMessage('')
    setSuccessMessage('')
    try {
      const response = await fetch(labelURL(selectedLabel), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (!response.ok) {
        throw await responseError(response, 'update')
      }

      const updated = await response.json()
      const updatedLabels = sortLabels(
        labels.map((label) => (label.id === updated.id ? updated : label))
      )
      setLabels(updatedLabels)
      setDraft(editableLabel(updated))
      setSuccessMessage(`Label "${updated.label_title}" saved successfully.`)
    } catch (error) {
      setErrorMessage(error.message)
    } finally {
      setSaving(false)
    }
  }

  const refreshAfterDelete = (deletedIndex) => {
    deleteTimer.current = setTimeout(async () => {
      deleteTimer.current = null
      try {
        const refreshed = await loadLabels()
        const nextLabel = refreshed[deletedIndex] || refreshed[0]
        setDraft(nextLabel ? editableLabel(nextLabel) : null)
        setLabels(refreshed)
        if (nextLabel) {
          navigate(labelEditorPath(nextLabel.id), { replace: true })
        } else {
          navigate('/labels/edit', { replace: true })
        }
        setSuccessMessage('')
      } catch (error) {
        setSuccessMessage('')
        setDraft(null)
        setErrorMessage(
          `Label was deleted, but the label list could not be refreshed: ${error.message}`
        )
      } finally {
        setDeletePending(false)
      }
    }, DELETE_SUCCESS_DURATION_MS)
  }

  const deleteLabel = async () => {
    if (!selectedLabel) return

    const deletedIndex = labels.findIndex(
      (label) => label.id === selectedLabel.id
    )
    setDeleting(true)
    setErrorMessage('')
    setSuccessMessage('')
    try {
      const response = await fetch(labelURL(selectedLabel), {
        method: 'DELETE',
      })
      if (!response.ok) {
        throw await responseError(response, 'delete')
      }

      setDeleteDialogOpen(false)
      setDeletePending(true)
      setSuccessMessage(
        `Label "${selectedLabel.label_title}" deleted successfully. The next label will load in 10 seconds.`
      )
      refreshAfterDelete(deletedIndex)
    } catch (error) {
      setDeleteDialogOpen(false)
      setErrorMessage(error.message)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <Container maxWidth="md" sx={{ py: 4 }}>
      <Stack spacing={3}>
        <Box display="flex" alignItems="center" justifyContent="space-between">
          <Typography variant="h4">Edit Failure Label</Typography>
          {draft && (
            <Tooltip title="Delete label">
              <span>
                <Button
                  color="error"
                  variant="outlined"
                  startIcon={<Delete />}
                  onClick={() => setDeleteDialogOpen(true)}
                  disabled={saving || deleting || deletePending}
                  aria-label={`Delete label ${draft.id}`}
                >
                  Delete
                </Button>
              </span>
            </Tooltip>
          )}
        </Box>

        {errorMessage && <Alert severity="error">{errorMessage}</Alert>}
        {successMessage && <Alert severity="success">{successMessage}</Alert>}

        {loading ? (
          <Box display="flex" justifyContent="center">
            <CircularProgress aria-label="Loading labels" />
          </Box>
        ) : labels.length === 0 && !deletePending ? (
          <Alert severity="info">No label definitions are available.</Alert>
        ) : (
          <Stack spacing={3}>
            <FormControl fullWidth disabled={deletePending}>
              <InputLabel id="label-selector-label">Label</InputLabel>
              <Select
                labelId="label-selector-label"
                label="Label"
                value={
                  labels.some((label) => label.id === draft?.id) ? draft.id : ''
                }
                onChange={(event) => selectLabel(event.target.value)}
              >
                {labels.map((label) => (
                  <MenuItem key={label.id} value={label.id}>
                    {label.label_title} ({label.id})
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            {draft ? (
              <Stack spacing={2}>
                <TextField
                  label="Label ID"
                  value={draft.id}
                  disabled
                  fullWidth
                />
                <TextField
                  label="Label title"
                  value={draft.label_title}
                  onChange={(event) =>
                    updateDraft('label_title', event.target.value)
                  }
                  error={!draft.label_title.trim() || duplicateTitle}
                  helperText={
                    duplicateTitle
                      ? 'Label title must be unique.'
                      : 'Required. The immutable ID is shown above.'
                  }
                  required
                  fullWidth
                  disabled={deletePending}
                />
                <MarkdownEditor
                  label="Explanation"
                  value={draft.explanation}
                  onChange={(event) =>
                    updateDraft('explanation', event.target.value)
                  }
                  helperText="Markdown is supported."
                  minRows={5}
                  size="medium"
                  disabled={deletePending}
                />
                <Autocomplete
                  multiple
                  freeSolo
                  autoSelect
                  options={[]}
                  value={draft.bugs}
                  onChange={(event, values) =>
                    updateDraft('bugs', normalizeJiraKeys(values))
                  }
                  disabled={deletePending}
                  renderInput={(params) => (
                    <TextField
                      {...params}
                      label="Associated Jira issues"
                      placeholder="OCPBUGS-12345"
                      error={invalidBugs.length > 0}
                      helperText={
                        invalidBugs.length > 0
                          ? `Invalid Jira key: ${invalidBugs.join(', ')}`
                          : 'Press Enter after each Jira issue key.'
                      }
                    />
                  )}
                />
                <FormControl fullWidth disabled={deletePending}>
                  <InputLabel id="display-contexts-label">
                    Hide in display contexts
                  </InputLabel>
                  <Select
                    labelId="display-contexts-label"
                    label="Hide in display contexts"
                    multiple
                    value={draft.hide_display_contexts}
                    onChange={(event) =>
                      updateDraft('hide_display_contexts', event.target.value)
                    }
                    renderValue={(selected) => (
                      <Stack direction="row" spacing={0.5} flexWrap="wrap">
                        {selected.map((value) => (
                          <Chip
                            key={value}
                            label={
                              DISPLAY_CONTEXTS.find(
                                (context) => context.value === value
                              )?.label || value
                            }
                            size="small"
                          />
                        ))}
                      </Stack>
                    )}
                  >
                    {DISPLAY_CONTEXTS.map((context) => (
                      <MenuItem key={context.value} value={context.value}>
                        <Checkbox
                          checked={draft.hide_display_contexts.includes(
                            context.value
                          )}
                        />
                        {context.label}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <Box display="flex" justifyContent="flex-end">
                  <Button
                    variant="contained"
                    onClick={saveLabel}
                    disabled={
                      !validDraft || saving || deleting || deletePending
                    }
                    startIcon={saving ? <CircularProgress size={18} /> : null}
                  >
                    {saving ? 'Saving...' : 'Save label'}
                  </Button>
                </Box>
              </Stack>
            ) : (
              <Alert severity="warning">
                The selected label was not found.
              </Alert>
            )}
          </Stack>
        )}
      </Stack>

      <Dialog
        open={deleteDialogOpen}
        onClose={() => !deleting && setDeleteDialogOpen(false)}
      >
        <DialogTitle>Delete failure label?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {`Delete label "${draft?.label_title}" (${draft?.id})? This action cannot be undone.`}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setDeleteDialogOpen(false)}
            disabled={deleting}
          >
            Cancel
          </Button>
          <Button
            color="error"
            variant="contained"
            onClick={deleteLabel}
            disabled={deleting}
            startIcon={deleting ? <CircularProgress size={18} /> : <Delete />}
          >
            {deleting ? 'Deleting...' : 'Delete label'}
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  )
}
