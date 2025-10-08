import {
  Chip,
  CircularProgress,
  IconButton,
  Paper,
  TextField,
  Tooltip,
} from '@mui/material'
import { makeStyles } from '@mui/styles'
import {
  Refresh as RefreshIcon,
  Send as SendIcon,
  Stop as StopIcon,
} from '@mui/icons-material'
import { validateMessage } from './chatUtils'
import PropTypes from 'prop-types'
import React, { useEffect, useRef, useState } from 'react'

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
}))

const EXAMPLE_QUERIES = [
  'What incidents are currently on-going?',
  'What is the latest 4.20 payload?',
  'When did 4.16 go GA?',
  'Why did [prow job ID] fail?',
]

export default function ChatInput({
  onSendMessage,
  disabled = false,
  isConnected = false,
  isTyping = false,
  onRetry,
  placeholder = 'Ask about OpenShift releases, job failures, or payload status...',
  contextChip = null,
  personaChip = null,
  suggestedQuestions = null,
}) {
  const classes = useStyles()
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const textFieldRef = useRef(null)

  // Use suggestedQuestions if provided, otherwise use default EXAMPLE_QUERIES
  const displayQuestions = suggestedQuestions || EXAMPLE_QUERIES

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
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      handleSendMessage()
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
        <div className={classes.statusChips}>
          <Chip
            size="small"
            label={getConnectionStatusText()}
            color={getConnectionStatusColor()}
            variant="outlined"
            className={classes.connectionStatus}
            icon={isTyping ? <CircularProgress size={12} /> : undefined}
          />
          {contextChip}
          {personaChip}
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
        <div className={classes.suggestions}>
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
          onKeyPress={handleKeyPress}
          placeholder={placeholder}
          disabled={disabled}
          error={!!error}
          helperText={error}
          variant="outlined"
          size="small"
        />

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
              onClick={isTyping ? undefined : handleSendMessage}
              disabled={!canSend && !isTyping}
              color="primary"
            >
              {isTyping ? <StopIcon /> : <SendIcon />}
            </IconButton>
          </span>
        </Tooltip>
      </div>
    </Paper>
  )
}

ChatInput.propTypes = {
  onSendMessage: PropTypes.func.isRequired,
  disabled: PropTypes.bool,
  isConnected: PropTypes.bool,
  isTyping: PropTypes.bool,
  onRetry: PropTypes.func,
  placeholder: PropTypes.string,
  contextChip: PropTypes.node,
  personaChip: PropTypes.node,
  suggestedQuestions: PropTypes.arrayOf(PropTypes.string),
}
