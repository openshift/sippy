import {
  Chip,
  CircularProgress,
  IconButton,
  List,
  ListItem,
  ListItemText,
  Menu,
  MenuItem,
  Paper,
  Popper,
  TextField,
  Tooltip,
} from '@mui/material'
import {
  Code as CodeIcon,
  Masks as MasksIcon,
  Refresh as RefreshIcon,
  Send as SendIcon,
  Stop as StopIcon,
} from '@mui/icons-material'
import { CONNECTION_STATES } from './store/webSocketSlice'
import { humanize, validateMessage } from './chatUtils'
import { makeStyles } from '@mui/styles'
import {
  useConnectionState,
  usePersonas,
  usePrompts,
  useSettings,
  useWebSocketActions,
} from './store/useChatStore'
import PropTypes from 'prop-types'
import React, { useEffect, useRef, useState } from 'react'
import SlashCommandModal from './SlashCommandModal'

const useStyles = makeStyles((theme) => ({
  inputContainer: {
    padding: theme.spacing(2),
    borderTop: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
  },
  inputBox: {
    display: 'flex',
    gap: theme.spacing(1),
    alignItems: 'flex-end',
  },
  textField: {
    flex: 1,
    '& .MuiOutlinedInput-root': {
      paddingRight: theme.spacing(1),
    },
  },
  sendButton: {
    padding: theme.spacing(1),
    '&.disabled': {
      opacity: 0.5,
    },
  },
  statusContainer: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: theme.spacing(1),
    minHeight: 24,
  },
  statusChips: {
    display: 'flex',
    gap: theme.spacing(1),
    alignItems: 'center',
  },
  connectionStatus: {
    fontSize: '0.75rem',
  },
  characterCount: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    '&.warning': {
      color: theme.palette.warning.main,
    },
    '&.error': {
      color: theme.palette.error.main,
    },
  },
  suggestions: {
    display: 'flex',
    gap: theme.spacing(1),
    marginBottom: theme.spacing(1),
    flexWrap: 'wrap',
  },
  suggestionChip: {
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  slashCommandPopper: {
    zIndex: theme.zIndex.modal + 1,
  },
  slashCommandList: {
    maxHeight: 300,
    overflow: 'auto',
  },
  slashCommandItem: {
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  commandMenuButton: {
    padding: theme.spacing(1),
  },
}))

const EXAMPLE_QUERIES = [
  'What incidents are currently on-going?',
  'What is the latest 4.20 payload?',
  'When did 4.16 go GA?',
  'Why did [prow job ID] fail?',
]

export default function ChatInput({
  onSendMessage,
  onRetry,
  placeholder = 'Ask about OpenShift releases, job failures, or payload status...',
  pageContext = null,
  suggestedQuestions = null,
}) {
  const classes = useStyles()
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const textFieldRef = useRef(null)

  // Slash command state
  const [showSlashCommands, setShowSlashCommands] = useState(false)
  const [slashCommandFilter, setSlashCommandFilter] = useState('')
  const [selectedPrompt, setSelectedPrompt] = useState(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [commandMenuAnchor, setCommandMenuAnchor] = useState(null)
  const [selectedCommandIndex, setSelectedCommandIndex] = useState(0)

  const { settings } = useSettings()
  const { personas } = usePersonas()
  const { prompts } = usePrompts()
  const { connectionState, isTyping } = useConnectionState()
  const { stopGeneration } = useWebSocketActions()

  const isConnected = connectionState === CONNECTION_STATES.CONNECTED
  const disabled = !isConnected

  const displayQuestions = suggestedQuestions || EXAMPLE_QUERIES

  // Filter prompts based on slash command input
  const filteredPrompts = prompts.filter((prompt) =>
    prompt.name.toLowerCase().includes(slashCommandFilter.toLowerCase())
  )

  const getContextDisplay = () => {
    if (!pageContext?.page) return null
    return humanize(pageContext.page)
  }

  // Reset selected command index when filtered prompts change
  useEffect(() => {
    setSelectedCommandIndex(0)
  }, [filteredPrompts.length, slashCommandFilter])

  // Focus input on mount
  useEffect(() => {
    if (textFieldRef.current && isConnected) {
      textFieldRef.current.focus()
    }
  }, [isConnected])

  const handleMessageChange = (event) => {
    const value = event.target.value
    setMessage(value)

    // Clear error when user starts typing
    if (error) {
      setError('')
    }

    // Detect slash commands
    if (value.startsWith('/')) {
      const command = value.slice(1)
      setSlashCommandFilter(command)
      setShowSlashCommands(true)
    } else {
      setShowSlashCommands(false)
      setSlashCommandFilter('')
    }
  }

  const handlePromptSelect = (prompt) => {
    setSelectedPrompt(prompt)
    setModalOpen(true)
    setShowSlashCommands(false)
    setMessage('')
  }

  const handleModalSubmit = (renderedPrompt) => {
    const success = onSendMessage(renderedPrompt)
    if (success) {
      setMessage('')
      setError('')
      setModalOpen(false)
      setSelectedPrompt(null)
    }
  }

  const handleModalClose = () => {
    setModalOpen(false)
    setSelectedPrompt(null)
  }

  const handleCommandMenuOpen = (event) => {
    setCommandMenuAnchor(event.currentTarget)
  }

  const handleCommandMenuClose = () => {
    setCommandMenuAnchor(null)
  }

  const handleCommandMenuSelect = (prompt) => {
    handlePromptSelect(prompt)
    handleCommandMenuClose()
  }

  const handleSendMessage = () => {
    const validation = validateMessage(message)

    if (!validation.valid) {
      setError(validation.error)
      return
    }

    if (!isConnected) {
      setError('Not connected to chat service')
      return
    }

    const success = onSendMessage(message.trim())
    if (success) {
      setMessage('')
      setError('')
    }
  }

  const handleKeyPress = (event) => {
    // Handle slash command navigation
    if (showSlashCommands && filteredPrompts.length > 0) {
      if (event.key === 'Tab') {
        event.preventDefault()
        if (event.shiftKey) {
          // Shift+Tab: move to previous command
          setSelectedCommandIndex((prev) =>
            prev <= 0 ? filteredPrompts.length - 1 : prev - 1
          )
        } else {
          // Tab: move to next command
          setSelectedCommandIndex((prev) =>
            prev >= filteredPrompts.length - 1 ? 0 : prev + 1
          )
        }
        return
      }

      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault()
        // Select the highlighted command
        if (filteredPrompts[selectedCommandIndex]) {
          handlePromptSelect(filteredPrompts[selectedCommandIndex])
        }
        return
      }

      if (event.key === 'Escape') {
        event.preventDefault()
        setShowSlashCommands(false)
        setMessage('')
        return
      }
    }

    // Normal message handling
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      // Only send if not typing (same logic as button)
      if (!isTyping) {
        handleSendMessage()
      }
    }
  }

  const handleSuggestionClick = (suggestion) => {
    setMessage(suggestion)
    textFieldRef.current?.focus()
  }

  const getCharacterCountClass = () => {
    const length = message.length
    if (length > 9000) return 'error'
    if (length > 8000) return 'warning'
    return ''
  }

  const getConnectionStatusColor = () => {
    if (!isConnected) return 'error'
    if (isTyping) return 'warning'
    return 'success'
  }

  const getConnectionStatusText = () => {
    if (!isConnected) return 'Disconnected'
    if (isTyping) return 'Sippy is thinking...'
    return 'Connected'
  }

  const canSend =
    message.trim().length > 0 && isConnected && !disabled && !isTyping

  return (
    <Paper className={classes.inputContainer} elevation={3}>
      {/* Status bar */}
      <div className={classes.statusContainer}>
        <div className={classes.statusChips} data-tour="status-area">
          <Chip
            size="small"
            label={getConnectionStatusText()}
            color={getConnectionStatusColor()}
            variant="outlined"
            className={classes.connectionStatus}
            icon={isTyping ? <CircularProgress size={12} /> : undefined}
          />
          {pageContext && getContextDisplay() && (
            <Tooltip title={`Context: ${getContextDisplay()}`}>
              <Chip
                label={getContextDisplay()}
                size="small"
                color="primary"
                variant="outlined"
              />
            </Tooltip>
          )}
          {personas.length > 0 && settings.persona !== 'default' && (
            <Tooltip
              title={
                personas.find((p) => p.id === settings.persona)?.description ||
                'Custom persona'
              }
            >
              <Chip
                icon={<MasksIcon />}
                label={
                  personas.find((p) => p.id === settings.persona)?.name ||
                  humanize(settings.persona)
                }
                size="small"
                color="secondary"
                variant="outlined"
              />
            </Tooltip>
          )}
          {onRetry && !isConnected && (
            <Tooltip title="Retry connection">
              <IconButton size="small" onClick={onRetry}>
                <RefreshIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          )}
        </div>

        <span
          className={`${classes.characterCount} ${getCharacterCountClass()}`}
        >
          {message.length}/10000
        </span>
      </div>

      {/* Example suggestions (show when input is empty) */}
      {message.length === 0 && (
        <div className={classes.suggestions} data-tour="sample-questions">
          {displayQuestions.slice(0, 5).map((suggestion, index) => (
            <Chip
              key={index}
              label={suggestion}
              size="small"
              variant="outlined"
              className={classes.suggestionChip}
              onClick={() => handleSuggestionClick(suggestion)}
            />
          ))}
        </div>
      )}

      {/* Input box */}
      <div className={classes.inputBox}>
        <TextField
          ref={textFieldRef}
          className={classes.textField}
          multiline
          maxRows={4}
          value={message}
          onChange={handleMessageChange}
          onKeyDown={handleKeyPress}
          placeholder={placeholder}
          disabled={disabled}
          error={!!error}
          helperText={error}
          variant="outlined"
          size="small"
        />

        {/* Slash command autocomplete popper */}
        <Popper
          open={showSlashCommands && filteredPrompts.length > 0}
          anchorEl={textFieldRef.current}
          placement="top-start"
          className={classes.slashCommandPopper}
        >
          <Paper elevation={4}>
            <List className={classes.slashCommandList}>
              {filteredPrompts.map((prompt, index) => (
                <ListItem
                  key={prompt.name}
                  className={classes.slashCommandItem}
                  onClick={() => handlePromptSelect(prompt)}
                  selected={index === selectedCommandIndex}
                >
                  <ListItemText
                    primary={`/${prompt.name}`}
                    secondary={prompt.description}
                  />
                </ListItem>
              ))}
            </List>
          </Paper>
        </Popper>

        {/* Command menu button */}
        <Tooltip title="Insert prompt command">
          <IconButton
            className={classes.commandMenuButton}
            onClick={handleCommandMenuOpen}
            disabled={disabled}
            color="primary"
          >
            <CodeIcon />
          </IconButton>
        </Tooltip>

        <Tooltip
          title={
            canSend
              ? 'Send message'
              : isTyping
              ? 'Stop generation'
              : 'Send message'
          }
        >
          <span>
            <IconButton
              className={`${classes.sendButton} ${
                !canSend && !isTyping ? 'disabled' : ''
              }`}
              onClick={isTyping ? stopGeneration : handleSendMessage}
              disabled={!canSend && !isTyping}
              color="primary"
            >
              {isTyping ? <StopIcon /> : <SendIcon />}
            </IconButton>
          </span>
        </Tooltip>
      </div>

      {/* Command menu */}
      <Menu
        anchorEl={commandMenuAnchor}
        open={Boolean(commandMenuAnchor)}
        onClose={handleCommandMenuClose}
      >
        {prompts.length === 0 ? (
          <MenuItem disabled>No prompts available</MenuItem>
        ) : (
          prompts.map((prompt) => (
            <MenuItem
              key={prompt.name}
              onClick={() => handleCommandMenuSelect(prompt)}
            >
              <ListItemText
                primary={`/${prompt.name}`}
                secondary={prompt.description}
              />
            </MenuItem>
          ))
        )}
      </Menu>

      {/* Slash command modal */}
      <SlashCommandModal
        open={modalOpen}
        onClose={handleModalClose}
        prompt={selectedPrompt}
        onSubmit={handleModalSubmit}
      />
    </Paper>
  )
}

ChatInput.propTypes = {
  onSendMessage: PropTypes.func.isRequired,
  onRetry: PropTypes.func,
  placeholder: PropTypes.string,
  pageContext: PropTypes.shape({
    page: PropTypes.string,
    url: PropTypes.string,
    data: PropTypes.object,
    instructions: PropTypes.string,
  }),
  suggestedQuestions: PropTypes.arrayOf(PropTypes.string),
}
