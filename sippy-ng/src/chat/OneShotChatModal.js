import {
  CircularProgress,
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  Typography,
} from '@mui/material'
import { Close as CloseIcon } from '@mui/icons-material'
import {
  createMessage,
  getChatWebSocketUrl,
  MESSAGE_TYPES,
  parseWebSocketMessage,
  WEBSOCKET_STATES,
} from './chatUtils'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useCallback, useEffect, useRef, useState } from 'react'
import ThinkingStep from './ThinkingStep'

const useStyles = makeStyles((theme) => ({
  dialogPaper: {
    height: '500px',
    display: 'flex',
    flexDirection: 'column',
  },
  dialogContent: {
    display: 'flex',
    flexDirection: 'column',
    padding: 0,
    flex: 1,
    overflow: 'hidden',
    '&:first-child': {
      paddingTop: 0,
    },
  },
  messagesContainer: {
    flex: 1,
    overflowY: 'auto',
    padding: theme.spacing(2),
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(2),
  },
  messageWrapper: {
    marginBottom: theme.spacing(1),
  },
  currentThinking: {
    margin: theme.spacing(1, 0),
    padding: theme.spacing(1),
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    borderRadius: theme.shape.borderRadius,
  },
  statusContainer: {
    padding: theme.spacing(2),
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: theme.spacing(2),
  },
  statusText: {
    textAlign: 'center',
    color: theme.palette.text.secondary,
  },
}))

/**
 * One-shot chat modal that makes a single request, shows thinking steps,
 * and returns the result via callback
 */
export default function OneShotChatModal({
  open,
  onClose,
  prompt,
  pageContext,
  onResult,
  title = 'Generating...',
}) {
  const classes = useStyles()
  const [connectionState, setConnectionState] = useState(
    WEBSOCKET_STATES.CLOSED
  )
  const [thinkingSteps, setThinkingSteps] = useState([])
  const [currentThinking, setCurrentThinking] = useState(null)
  const [error, setError] = useState(null)
  const [result, setResult] = useState(null)

  const wsRef = useRef(null)
  const currentIterationRef = useRef(0)
  const hasStartedRef = useRef(false)
  const messagesEndRef = useRef(null)
  const lastMessageRef = useRef(null)

  // Connect to WebSocket and send message
  const startRequest = useCallback(() => {
    if (hasStartedRef.current || !prompt) {
      return
    }

    hasStartedRef.current = true

    try {
      const wsUrl = getChatWebSocketUrl()
      console.log('OneShotChat: Connecting to WebSocket:', wsUrl)

      wsRef.current = new WebSocket(wsUrl)
      setConnectionState(WEBSOCKET_STATES.CONNECTING)
      setError(null)

      wsRef.current.onopen = () => {
        console.log('OneShotChat: WebSocket connected')
        setConnectionState(WEBSOCKET_STATES.OPEN)

        // Send the request immediately
        const payload = {
          message: prompt,
          chat_history: [],
          show_thinking: true,
          persona: 'default',
          page_context: pageContext,
        }

        console.log('OneShotChat: Sending request:', payload)
        wsRef.current.send(JSON.stringify(payload))
      }

      wsRef.current.onmessage = (event) => {
        const message = parseWebSocketMessage(event.data)
        if (!message) return

        handleWebSocketMessage(message)
      }

      wsRef.current.onclose = (event) => {
        console.log('OneShotChat: WebSocket closed:', event.code, event.reason)
        setConnectionState(WEBSOCKET_STATES.CLOSED)
      }

      wsRef.current.onerror = (error) => {
        console.error('OneShotChat: WebSocket error:', error)
        setError('Connection error occurred')
        setConnectionState(WEBSOCKET_STATES.CLOSED)
      }
    } catch (err) {
      console.error('OneShotChat: Failed to create WebSocket connection:', err)
      setError('Failed to connect to chat service')
      setConnectionState(WEBSOCKET_STATES.CLOSED)
    }
  }, [prompt, pageContext])

  // Handle incoming WebSocket messages
  const handleWebSocketMessage = useCallback((message) => {
    switch (message.type) {
      case MESSAGE_TYPES.THINKING_STEP:
        handleThinkingStep(message.data)
        break

      case MESSAGE_TYPES.FINAL_RESPONSE:
        handleFinalResponse(message.data)
        break

      case MESSAGE_TYPES.ERROR:
        handleError(message.data)
        break

      default:
        console.warn('OneShotChat: Unknown message type:', message.type)
    }
  }, [])

  // Handle thinking step updates
  const handleThinkingStep = useCallback((data) => {
    if (data.complete) {
      // Thinking step completed - add to list
      setThinkingSteps((prev) => {
        const existing = prev.find(
          (step) =>
            step.data?.step_number === data.step_number &&
            step.data?.iteration === currentIterationRef.current
        )

        if (existing) {
          return prev.map((step) =>
            step.id === existing.id
              ? {
                  ...step,
                  data: {
                    ...step.data,
                    ...data,
                    iteration: currentIterationRef.current,
                  },
                }
              : step
          )
        } else {
          return [
            ...prev,
            createMessage(MESSAGE_TYPES.THINKING_STEP, '', {
              data: { ...data, iteration: currentIterationRef.current },
            }),
          ]
        }
      })
      setCurrentThinking(null)
    } else {
      // Thinking step in progress
      setCurrentThinking({ ...data, iteration: currentIterationRef.current })
    }
  }, [])

  // Handle final response
  const handleFinalResponse = useCallback(
    (data) => {
      console.log('OneShotChat: Final response received:', data.response)
      setCurrentThinking(null)
      setResult(data.response)

      // Call the callback with the result
      if (onResult) {
        onResult(data.response)
      }

      // Close the WebSocket
      if (wsRef.current) {
        wsRef.current.close(1000, 'Request completed')
      }
    },
    [onResult]
  )

  // Handle error messages
  const handleError = useCallback((data) => {
    console.error('OneShotChat: Error received:', data.error)
    setCurrentThinking(null)
    setError(data.error)

    // Close the WebSocket
    if (wsRef.current) {
      wsRef.current.close(1000, 'Error occurred')
    }
  }, [])

  // Start request when modal opens
  useEffect(() => {
    if (open && prompt) {
      startRequest()
    }

    // Cleanup on unmount or close
    return () => {
      if (wsRef.current) {
        wsRef.current.close(1000, 'Modal closed')
        wsRef.current = null
      }
    }
  }, [open, prompt, startRequest])

  // Reset state when modal is closed
  useEffect(() => {
    if (!open) {
      setConnectionState(WEBSOCKET_STATES.CLOSED)
      setThinkingSteps([])
      setCurrentThinking(null)
      setError(null)
      setResult(null)
      hasStartedRef.current = false
      currentIterationRef.current = 0
    }
  }, [open])

  // Auto-scroll to show the latest message when new thinking steps arrive
  useEffect(() => {
    if (lastMessageRef.current) {
      // Scroll to the top of the last message, not the bottom
      lastMessageRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
      })
    }
  }, [thinkingSteps, currentThinking])

  const handleClose = () => {
    if (wsRef.current) {
      wsRef.current.close(1000, 'User closed modal')
    }
    onClose()
  }

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        className: classes.dialogPaper,
      }}
    >
      <DialogTitle
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          paddingRight: 6,
        }}
      >
        {title}
        {!error && !result && (
          <CircularProgress size={20} sx={{ marginLeft: 1 }} />
        )}
        <IconButton
          aria-label="close"
          onClick={handleClose}
          sx={{ position: 'absolute', right: 8, top: 8 }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent className={classes.dialogContent}>
        {error ? (
          <div className={classes.statusContainer}>
            <Typography color="error">{error}</Typography>
          </div>
        ) : result ? (
          <div className={classes.statusContainer}>
            <Typography variant="body1" color="success.main">
              ✓ Generation complete
            </Typography>
          </div>
        ) : connectionState === WEBSOCKET_STATES.CONNECTING ? (
          <div className={classes.statusContainer}>
            <CircularProgress />
            <Typography className={classes.statusText}>
              Connecting...
            </Typography>
          </div>
        ) : (
          <div className={classes.messagesContainer}>
            {thinkingSteps.map((step, index) => {
              const isLastStep = index === thinkingSteps.length - 1
              return (
                <div
                  key={index}
                  className={classes.messageWrapper}
                  ref={isLastStep && !currentThinking ? lastMessageRef : null}
                >
                  <ThinkingStep data={step.data} defaultExpanded={false} />
                </div>
              )
            })}

            {currentThinking && (
              <div className={classes.currentThinking} ref={lastMessageRef}>
                <ThinkingStep data={currentThinking} isInProgress={true} />
              </div>
            )}

            <div ref={messagesEndRef} />
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

OneShotChatModal.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  prompt: PropTypes.string.isRequired,
  pageContext: PropTypes.object,
  onResult: PropTypes.func.isRequired,
  title: PropTypes.string,
}
