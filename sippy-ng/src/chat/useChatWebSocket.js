import {
  createMessage,
  formatChatHistoryForAPI,
  getChatWebSocketUrl,
  MESSAGE_TYPES,
  parseWebSocketMessage,
  WEBSOCKET_STATES,
} from './chatUtils'
import { useCallback, useEffect, useRef, useState } from 'react'

export function useChatWebSocket(settings = {}, pageContext = null) {
  const [messages, setMessages] = useState([])
  const [connectionState, setConnectionState] = useState(
    WEBSOCKET_STATES.CLOSED
  )
  const [currentThinking, setCurrentThinking] = useState(null)
  const [error, setError] = useState(null)
  const [sessionRated, setSessionRated] = useState(false)
  const [isTyping, setIsTyping] = useState(false)

  const wsRef = useRef(null)
  const reconnectTimeoutRef = useRef(null)
  const reconnectAttemptsRef = useRef(0)
  const currentIterationRef = useRef(0)
  const pageContextRef = useRef(pageContext)
  const maxReconnectAttempts = 5

  // Update pageContext ref when it changes
  useEffect(() => {
    console.log('useChatWebSocket: updating pageContextRef to:', pageContext)
    pageContextRef.current = pageContext
  }, [pageContext])

  // Connect to WebSocket
  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      return
    }

    try {
      const wsUrl = getChatWebSocketUrl()
      console.log('Connecting to WebSocket:', wsUrl)

      wsRef.current = new WebSocket(wsUrl)
      setConnectionState(WEBSOCKET_STATES.CONNECTING)
      setError(null)

      wsRef.current.onopen = () => {
        console.log('WebSocket connected')
        setConnectionState(WEBSOCKET_STATES.OPEN)
        reconnectAttemptsRef.current = 0
        setError(null)
      }

      wsRef.current.onmessage = (event) => {
        const message = parseWebSocketMessage(event.data)
        if (!message) return

        handleWebSocketMessage(message)
      }

      wsRef.current.onclose = (event) => {
        console.log('WebSocket closed:', event.code, event.reason)
        setConnectionState(WEBSOCKET_STATES.CLOSED)
        setIsTyping(false)
        setCurrentThinking(null)

        // Attempt to reconnect if not a normal closure
        if (
          event.code !== 1000 &&
          reconnectAttemptsRef.current < maxReconnectAttempts
        ) {
          const delay = Math.min(
            1000 * Math.pow(2, reconnectAttemptsRef.current),
            30000
          )
          console.log(
            `Reconnecting in ${delay}ms (attempt ${
              reconnectAttemptsRef.current + 1
            })`
          )

          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectAttemptsRef.current++
            connect()
          }, delay)
        }
      }

      wsRef.current.onerror = (error) => {
        console.error('WebSocket error:', error)
        setError('Connection error occurred')
      }
    } catch (err) {
      console.error('Failed to create WebSocket connection:', err)
      setError('Failed to connect to chat service')
      setConnectionState(WEBSOCKET_STATES.CLOSED)
    }
  }, [])

  // Disconnect WebSocket
  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }

    if (wsRef.current) {
      setConnectionState(WEBSOCKET_STATES.CLOSING)
      wsRef.current.close(1000, 'User disconnected')
      wsRef.current = null
    }
  }, [])

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
        console.warn('Unknown message type:', message.type)
    }
  }, [])

  // Handle thinking step updates
  const handleThinkingStep = useCallback((data) => {
    if (data.complete) {
      // Thinking step completed
      setMessages((prev) => {
        // Look for existing thinking step from the CURRENT iteration only
        const existing = prev.find(
          (msg) =>
            msg.type === MESSAGE_TYPES.THINKING_STEP &&
            msg.data?.step_number === data.step_number &&
            msg.data?.iteration === currentIterationRef.current
        )

        if (existing) {
          // Update existing thinking step from current iteration
          return prev.map((msg) =>
            msg.id === existing.id
              ? {
                  ...msg,
                  data: {
                    ...msg.data,
                    ...data,
                    iteration: currentIterationRef.current,
                  },
                }
              : msg
          )
        } else {
          // Add new thinking step with current iteration
          return [
            ...prev,
            createMessage(MESSAGE_TYPES.THINKING_STEP, '', {
              data: { ...data, iteration: currentIterationRef.current },
            }),
          ]
        }
      })

      setCurrentThinking(null)
      // Keep isTyping true - agent might still be working on next step or final response
    } else {
      // Thinking step in progress
      setIsTyping(true)
      setCurrentThinking({ ...data, iteration: currentIterationRef.current })
    }
  }, [])

  // Handle final response
  const handleFinalResponse = useCallback((data) => {
    setIsTyping(false)
    setCurrentThinking(null)

    const responseMessage = createMessage(
      MESSAGE_TYPES.ASSISTANT,
      data.response,
      {
        tools_used: data.tools_used,
        timestamp: data.timestamp,
        pageContext: pageContextRef.current,
      }
    )

    setMessages((prev) => [...prev, responseMessage])
  }, [])

  // Handle error messages
  const handleError = useCallback((data) => {
    setIsTyping(false)
    setCurrentThinking(null)

    const errorMessage = createMessage(MESSAGE_TYPES.ERROR, data.error, {
      timestamp: data.timestamp,
    })

    setMessages((prev) => [...prev, errorMessage])
    setError(data.error)
  }, [])

  // Send message
  const sendMessage = useCallback(
    (content) => {
      if (connectionState !== WEBSOCKET_STATES.OPEN) {
        setError('Not connected to chat service')
        return false
      }

      try {
        // Increment iteration counter for new conversation turn
        currentIterationRef.current += 1

        // Add user message to chat with page context
        const userMessage = createMessage(MESSAGE_TYPES.USER, content, {
          pageContext: pageContextRef.current,
        })
        setMessages((prev) => [...prev, userMessage])

        // Send to WebSocket
        const payload = {
          message: content,
          chat_history: formatChatHistoryForAPI(messages),
          show_thinking: settings.showThinking !== false,
          persona: settings.persona || 'default',
          page_context: pageContextRef.current,
        }

        console.log(
          'Sending message with page context:',
          pageContextRef.current
        )
        wsRef.current.send(JSON.stringify(payload))
        setError(null)
        setIsTyping(true) // Start typing indicator when message is sent
        return true
      } catch (err) {
        console.error('Failed to send message:', err)
        setError('Failed to send message')
        return false
      }
    },
    [connectionState, messages, settings.showThinking, settings.persona]
  )

  // Clear chat history
  const clearMessages = useCallback(() => {
    setMessages([])
    setCurrentThinking(null)
    setError(null)
    setIsTyping(false)
    setSessionRated(false)
    currentIterationRef.current = 0 // Reset iteration counter
  }, [])

  // Auto-connect on mount
  useEffect(() => {
    connect()

    return () => {
      disconnect()
    }
  }, [connect, disconnect])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
    }
  }, [])

  return {
    messages,
    connectionState,
    currentThinking,
    error,
    isTyping,
    sessionRated,
    setSessionRated,
    sendMessage,
    clearMessages,
    connect,
    disconnect,
    isConnected: connectionState === WEBSOCKET_STATES.OPEN,
  }
}
