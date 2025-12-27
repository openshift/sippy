import {
  Add as AddIcon,
  Close as CloseIcon,
  Code as CodeIcon,
  Computer as ComputerIcon,
  Delete as DeleteIcon,
  Edit as EditIcon,
  FileDownload as FileDownloadIcon,
  FileUpload as FileUploadIcon,
  Search as SearchIcon,
} from '@mui/icons-material'
import {
  Box,
  Button,
  Dialog,
  DialogContent,
  Divider,
  IconButton,
  InputAdornment,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  TextField,
  Toolbar,
  Typography,
} from '@mui/material'
import { extractYAMLFromText, promptToYAML } from './promptSchema'
import { makeStyles } from '@mui/styles'
import { usePrompts } from './store/useChatStore'
import CreatePromptDialog from './CreatePromptDialog'
import PromptEditor from './PromptEditor'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  dialog: {
    '& .MuiDialog-paper': {
      width: '90vw',
      maxWidth: 1400,
      height: '85vh',
      maxHeight: 900,
    },
  },
  dialogContent: {
    padding: 0,
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
  },
  toolbar: {
    borderBottom: `1px solid ${theme.palette.divider}`,
    padding: theme.spacing(2),
    gap: theme.spacing(2),
  },
  mainContent: {
    display: 'flex',
    flex: 1,
    overflow: 'hidden',
  },
  sidebar: {
    width: 320,
    borderRight: `1px solid ${theme.palette.divider}`,
    display: 'flex',
    flexDirection: 'column',
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.02)'
        : 'rgba(0, 0, 0, 0.02)',
  },
  sidebarHeader: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  promptList: {
    flex: 1,
    overflow: 'auto',
    padding: 0,
  },
  promptListItem: {
    borderBottom: `1px solid ${theme.palette.divider}`,
    '&.Mui-selected': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(144, 202, 249, 0.16)'
          : 'rgba(25, 118, 210, 0.08)',
    },
  },
  emptyState: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    height: '100%',
    padding: theme.spacing(4),
    textAlign: 'center',
    color: theme.palette.text.secondary,
  },
  detailPanel: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
  },
  detailHeader: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  detailContent: {
    flex: 1,
    overflow: 'auto',
    padding: theme.spacing(3),
  },
  promptPreview: {
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    fontFamily: 'monospace',
    fontSize: '0.875rem',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  },
  metadataRow: {
    display: 'flex',
    gap: theme.spacing(2),
    marginBottom: theme.spacing(1),
  },
}))

export default function PromptManagerModal({ open, onClose }) {
  const classes = useStyles()
  const {
    localPrompts,
    deleteLocalPrompt,
    exportLocalPromptsAsYAML,
    saveLocalPrompt,
  } = usePrompts()

  const [selectedPromptName, setSelectedPromptName] = useState(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editorPromptName, setEditorPromptName] = useState(null)
  const [editorInitialYAML, setEditorInitialYAML] = useState(null)

  // Filter prompts based on search
  const filteredPrompts = localPrompts.filter(
    (prompt) =>
      prompt.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      prompt.description?.toLowerCase().includes(searchQuery.toLowerCase())
  )

  const selectedPrompt = localPrompts.find((p) => p.name === selectedPromptName)

  const handlePromptSelect = (promptName) => {
    setSelectedPromptName(promptName)
  }

  const handleCreateNew = () => {
    setCreateDialogOpen(true)
  }

  const handleEdit = () => {
    if (selectedPrompt) {
      setEditorPromptName(selectedPrompt.name)
      setEditorInitialYAML(null)
      setEditorOpen(true)
    }
  }

  const handleDelete = () => {
    if (
      selectedPrompt &&
      window.confirm(
        `Are you sure you want to delete "${selectedPrompt.name}"?`
      )
    ) {
      deleteLocalPrompt(selectedPrompt.name)
      setSelectedPromptName(null)
    }
  }

  const handleYAMLGenerated = (aiResponse) => {
    const yamlBlocks = extractYAMLFromText(aiResponse)
    const yamlContent = yamlBlocks.length > 0 ? yamlBlocks[0] : aiResponse
    setEditorInitialYAML(yamlContent)
    setEditorPromptName(null)
    setEditorOpen(true)
  }

  const handleEditorClose = () => {
    setEditorOpen(false)
    setEditorPromptName(null)
    setEditorInitialYAML(null)
  }

  const handleExport = () => {
    if (!selectedPrompt) {
      return
    }

    // Export selected prompt with its name as filename
    const { createdAt, updatedAt, source, ...cleanPrompt } = selectedPrompt
    const yamlContent = promptToYAML(cleanPrompt)
    const dataStr =
      'data:text/yaml;charset=utf-8,' + encodeURIComponent(yamlContent)
    const a = document.createElement('a')
    a.href = dataStr
    a.download = `${selectedPrompt.name}.yaml`
    a.click()
  }

  const handleImport = () => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.yaml,.yml'
    input.onchange = (e) => {
      const file = e.target.files[0]
      if (file) {
        const reader = new FileReader()
        reader.onload = (event) => {
          try {
            const yamlContent = event.target.result
            // Split by document separator if multiple prompts
            const prompts = yamlContent.split(/\n---\n/)
            prompts.forEach((promptYAML) => {
              if (promptYAML.trim()) {
                setEditorInitialYAML(promptYAML.trim())
                setEditorPromptName(null)
                setEditorOpen(true)
              }
            })
          } catch (error) {
            alert(`Failed to import: ${error.message}`)
          }
        }
        reader.readAsText(file)
      }
    }
    input.click()
  }

  return (
    <>
      <Dialog
        open={open}
        onClose={onClose}
        maxWidth={false}
        className={classes.dialog}
      >
        <DialogContent className={classes.dialogContent}>
          {/* Toolbar */}
          <Toolbar className={classes.toolbar}>
            <Box display="flex" alignItems="center" gap={1} flex={1}>
              <CodeIcon color="primary" />
              <Typography variant="h6">Custom Prompt Manager</Typography>
              <Typography variant="body2" color="textSecondary">
                ({localPrompts.length} prompt
                {localPrompts.length !== 1 ? 's' : ''})
              </Typography>
            </Box>

            <Box display="flex" gap={1}>
              <Button
                startIcon={<AddIcon />}
                variant="contained"
                onClick={handleCreateNew}
                size="small"
              >
                Create New
              </Button>
              <Button
                startIcon={<FileDownloadIcon />}
                variant="outlined"
                onClick={handleExport}
                disabled={!selectedPrompt}
                size="small"
              >
                Export
              </Button>
              <Button
                startIcon={<FileUploadIcon />}
                variant="outlined"
                onClick={handleImport}
                size="small"
              >
                Import
              </Button>
              <IconButton onClick={onClose} size="small">
                <CloseIcon />
              </IconButton>
            </Box>
          </Toolbar>

          {/* Main Content */}
          <Box className={classes.mainContent}>
            {/* Sidebar */}
            <Box className={classes.sidebar}>
              <Box className={classes.sidebarHeader}>
                <TextField
                  placeholder="Search prompts..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  size="small"
                  fullWidth
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <SearchIcon fontSize="small" />
                      </InputAdornment>
                    ),
                  }}
                />
              </Box>

              <List className={classes.promptList}>
                {filteredPrompts.length === 0 ? (
                  <Box className={classes.emptyState}>
                    <CodeIcon
                      sx={{ fontSize: 64, mb: 2, opacity: 0.3 }}
                      color="disabled"
                    />
                    <Typography variant="body2">
                      {searchQuery
                        ? 'No prompts match your search'
                        : 'No custom prompts yet'}
                    </Typography>
                    {!searchQuery && (
                      <Button
                        startIcon={<AddIcon />}
                        onClick={handleCreateNew}
                        sx={{ mt: 2 }}
                      >
                        Create Your First Prompt
                      </Button>
                    )}
                  </Box>
                ) : (
                  filteredPrompts.map((prompt) => (
                    <ListItem
                      key={prompt.name}
                      disablePadding
                      className={classes.promptListItem}
                    >
                      <ListItemButton
                        selected={selectedPromptName === prompt.name}
                        onClick={() => handlePromptSelect(prompt.name)}
                      >
                        <ComputerIcon
                          fontSize="small"
                          sx={{ mr: 1.5, opacity: 0.7 }}
                        />
                        <ListItemText
                          primary={prompt.name}
                          secondary={prompt.description}
                          primaryTypographyProps={{
                            variant: 'body2',
                            fontWeight: 500,
                          }}
                          secondaryTypographyProps={{
                            variant: 'caption',
                            noWrap: true,
                          }}
                        />
                      </ListItemButton>
                    </ListItem>
                  ))
                )}
              </List>
            </Box>

            {/* Detail Panel */}
            <Box className={classes.detailPanel}>
              {!selectedPrompt ? (
                <Box className={classes.emptyState}>
                  <Typography variant="h6" gutterBottom>
                    Select a prompt to view details
                  </Typography>
                  <Typography variant="body2" color="textSecondary">
                    Choose a prompt from the list or create a new one
                  </Typography>
                </Box>
              ) : (
                <>
                  <Box className={classes.detailHeader}>
                    <Box>
                      <Typography variant="h6">
                        {selectedPrompt.name}
                      </Typography>
                      <Typography
                        variant="body2"
                        color="textSecondary"
                        sx={{ mt: 0.5 }}
                      >
                        {selectedPrompt.description}
                      </Typography>
                    </Box>
                    <Box display="flex" gap={1}>
                      <IconButton
                        onClick={handleEdit}
                        color="primary"
                        size="small"
                      >
                        <EditIcon />
                      </IconButton>
                      <IconButton
                        onClick={handleDelete}
                        color="error"
                        size="small"
                      >
                        <DeleteIcon />
                      </IconButton>
                    </Box>
                  </Box>

                  <Box className={classes.detailContent}>
                    {/* Metadata */}
                    <Typography variant="subtitle2" gutterBottom>
                      Metadata
                    </Typography>
                    <Box className={classes.metadataRow}>
                      <Typography variant="body2" color="textSecondary">
                        <strong>Created:</strong>{' '}
                        {selectedPrompt.createdAt
                          ? new Date(selectedPrompt.createdAt).toLocaleString()
                          : 'Unknown'}
                      </Typography>
                    </Box>
                    <Box className={classes.metadataRow}>
                      <Typography variant="body2" color="textSecondary">
                        <strong>Updated:</strong>{' '}
                        {selectedPrompt.updatedAt
                          ? new Date(selectedPrompt.updatedAt).toLocaleString()
                          : 'Unknown'}
                      </Typography>
                    </Box>

                    {/* Arguments */}
                    {selectedPrompt.arguments &&
                      selectedPrompt.arguments.length > 0 && (
                        <>
                          <Divider sx={{ my: 2 }} />
                          <Typography variant="subtitle2" gutterBottom>
                            Arguments ({selectedPrompt.arguments.length})
                          </Typography>
                          {selectedPrompt.arguments.map((arg, idx) => (
                            <Box
                              key={idx}
                              sx={{
                                mb: 1,
                                p: 1.5,
                                backgroundColor: (theme) =>
                                  theme.palette.mode === 'dark'
                                    ? 'rgba(255, 255, 255, 0.05)'
                                    : 'rgba(0, 0, 0, 0.02)',
                                borderRadius: 1,
                              }}
                            >
                              <Typography variant="body2" fontWeight={600}>
                                {arg.name}
                                {arg.required && (
                                  <Typography
                                    component="span"
                                    color="error"
                                    sx={{ ml: 0.5 }}
                                  >
                                    *
                                  </Typography>
                                )}
                                <Typography
                                  component="span"
                                  variant="caption"
                                  color="textSecondary"
                                  sx={{ ml: 1 }}
                                >
                                  ({arg.type || 'string'})
                                </Typography>
                              </Typography>
                              <Typography
                                variant="caption"
                                color="textSecondary"
                              >
                                {arg.description}
                              </Typography>
                            </Box>
                          ))}
                        </>
                      )}

                    {/* Prompt Template */}
                    <Divider sx={{ my: 2 }} />
                    <Typography variant="subtitle2" gutterBottom>
                      Prompt Template
                    </Typography>
                    <Box className={classes.promptPreview}>
                      {selectedPrompt.prompt}
                    </Box>

                    {/* YAML Preview */}
                    <Divider sx={{ my: 2 }} />
                    <Typography variant="subtitle2" gutterBottom>
                      YAML Source
                    </Typography>
                    <Box className={classes.promptPreview}>
                      {promptToYAML({
                        name: selectedPrompt.name,
                        description: selectedPrompt.description,
                        arguments: selectedPrompt.arguments,
                        prompt: selectedPrompt.prompt,
                        hide: selectedPrompt.hide,
                      })}
                    </Box>
                  </Box>
                </>
              )}
            </Box>
          </Box>
        </DialogContent>
      </Dialog>

      {/* Create Dialog */}
      <CreatePromptDialog
        open={createDialogOpen}
        onClose={() => setCreateDialogOpen(false)}
        onYAMLGenerated={handleYAMLGenerated}
      />

      {/* Editor */}
      <PromptEditor
        open={editorOpen}
        onClose={handleEditorClose}
        promptName={editorPromptName}
        initialYAML={editorInitialYAML}
      />
    </>
  )
}

PromptManagerModal.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
}
