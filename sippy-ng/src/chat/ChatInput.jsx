import {
  Chip,
  CircularProgress,
  IconButton,
  Paper,
  TextField,
  Tooltip,
} from '@mui/material'
import {
  Code as CodeIcon,
  Masks as MasksIcon,
  PlayArrow as PlayArrowIcon,
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
import SlashCommandSelector from './SlashCommandSelector'

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
  commandMenuButton: {
    padding: theme.spacing(1),
  },
}))

// Default suggestions - can be either questions (strings) or commands (objects)
const DEFAULT_SUGGESTIONS = [
  { prompt: 'payload-report', label: 'Payload Status Report' },
  { prompt: 'plot-job-results', label: 'Plot Job Results' },
  { prompt: 'job-run-analysis', label: 'Analyze a Job Run' },
  { prompt: 'jira-incidents', label: 'View Open Incidents' },
]

export default function ChatInput({
  onSendMessage,
  onRetry,
  placeholder = 'Ask about OpenShift releases, job failures, or payload status...',
  pageContext = null,
  suggestions = null,
}) {
  const classes = useStyles()
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const textFieldRef = useRef(null)

  // Slash command state
  const [showSlashCommands, setShowSlashCommands] = useState(false)
  const [selectedPrompt, setSelectedPrompt] = useState(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [commandMenuAnchor, setCommandMenuAnchor] = useState(null)
  const slashNavigationRef = useRef(null)

  const { settings } = useSettings()
  const { personas } = usePersonas()
  const { prompts, renderPrompt } = usePrompts()
  const { connectionState, isTyping } = useConnectionState()
  const { stopGeneration } = useWebSocketActions()

  const isConnected = connectionState === CONNECTION_STATES.CONNECTED
  const disabled = !isConnected

  const displaySuggestions = suggestions || DEFAULT_SUGGESTIONS

  const slashCommandFilter =
    message && message.startsWith('/') ? message.slice(1) : ''

  const getContextDisplay = () => {
    if (!pageContext?.page) return null
    return humanize(pageContext.page)
  }

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

    setShowSlashCommands(value.startsWith('/'))
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
    if (showSlashCommands && slashNavigationRef.current) {
      if (event.key === 'Tab') {
        event.preventDefault()
        if (event.shiftKey) {
          slashNavigationRef.current.movePrevious()
        } else {
          slashNavigationRef.current.moveNext()
        }
        return
      }

      if (event.key === 'ArrowDown') {
        event.preventDefault()
        slashNavigationRef.current.moveNext()
        return
      }

      if (event.key === 'ArrowUp') {
        event.preventDefault()
        slashNavigationRef.current.movePrevious()
        return
      }

      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault()
        slashNavigationRef.current.selectCurrent()
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

  const handleSuggestionClick = async (suggestion) => {
    // Handle plain text questions - send directly
    if (typeof suggestion === 'string') {
      onSendMessage(suggestion)
      return
    }

    // Handle command objects
    const command = suggestion

    // Find the prompt definition
    const prompt = prompts.find((p) => p.name === command.prompt)
    if (!prompt) {
      console.error(`Prompt '${command.prompt}' not found`)
      setError(`Command '${command.prompt}' not found`)
      return
    }

    // Check if prompt has any required arguments
    const hasRequiredArgs =
      prompt.arguments && prompt.arguments.some((arg) => arg.required)

    // Check if all required arguments are pre-filled
    const allRequiredArgsFilled =
      !hasRequiredArgs ||
      (command.args &&
        prompt.arguments.every(
          (arg) => !arg.required || command.args[arg.name] !== undefined
        ))

    // If no required arguments OR all required args are pre-filled, send directly
    if (allRequiredArgsFilled) {
      try {
        const rendered = await renderPrompt(prompt.name, command.args || {})
        onSendMessage(rendered)
      } catch (err) {
        console.error('Failed to render prompt:', err)
        setError(`Failed to render prompt '${prompt.name}': ${err.message}`)
      }
      return
    }

    // Otherwise, show the modal for user input
    setSelectedPrompt({ ...prompt, prefilledArgs: command.args })
    setModalOpen(true)
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

      {/* Suggestions (show when input is empty) */}
      {message.length === 0 && (
        <div className={classes.suggestions} data-tour="suggestions">
          {displaySuggestions.slice(0, 5).map((suggestion, index) => {
            // Check if it's a command object or plain question
            const isCommand = typeof suggestion === 'object'

            if (isCommand) {
              // Command chip with icon and tooltip
              return (
                <Tooltip
                  key={index}
                  title={`Run command: /${suggestion.prompt}`}
                  arrow
                >
                  <span>
                    <Chip
                      icon={<PlayArrowIcon />}
                      label={suggestion.label}
                      size="small"
                      variant="outlined"
                      className={classes.suggestionChip}
                      onClick={() => handleSuggestionClick(suggestion)}
                      disabled={isTyping}
                    />
                  </span>
                </Tooltip>
              )
            }

            // Plain question chip
            return (
              <Chip
                key={index}
                label={suggestion}
                size="small"
                variant="outlined"
                className={classes.suggestionChip}
                onClick={() => handleSuggestionClick(suggestion)}
                disabled={isTyping}
              />
            )
          })}
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

        {/* Slash command autocomplete */}
        <SlashCommandSelector
          anchorEl={textFieldRef.current}
          filterText={slashCommandFilter}
          open={showSlashCommands}
          onSelect={handlePromptSelect}
          onNavigate={(navigation) => {
            slashNavigationRef.current = navigation
          }}
          placement="top-start"
        />

        {/* Command menu button */}
        <Tooltip title="Insert prompt command">
          <span data-tour="command-button">
            <IconButton
              className={classes.commandMenuButton}
              onClick={handleCommandMenuOpen}
              disabled={disabled || isTyping}
              color="primary"
            >
              <CodeIcon />
            </IconButton>
          </span>
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
      <SlashCommandSelector
        anchorEl={commandMenuAnchor}
        filterText=""
        open={Boolean(commandMenuAnchor)}
        onSelect={handlePromptSelect}
        onClose={handleCommandMenuClose}
        placement="bottom-start"
      />

      {/* Slash command modal */}
      <SlashCommandModal
        open={modalOpen}
        onClose={handleModalClose}
        prompt={selectedPrompt}
        onSubmit={handleModalSubmit}
        disabled={isTyping}
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
  suggestions: PropTypes.arrayOf(
    PropTypes.oneOfType([
      PropTypes.string, // Plain text question
      PropTypes.shape({
        // Command object
        prompt: PropTypes.string.isRequired,
        label: PropTypes.string.isRequired,
        args: PropTypes.object,
      }),
    ])
  ),
}
