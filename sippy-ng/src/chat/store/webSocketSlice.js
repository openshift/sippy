import {
  createMessage,
  formatChatHistoryForAPI,
  getChatWebSocketUrl,
  MESSAGE_TYPES,
  parseWebSocketMessage,
} from '../chatUtils'

/**
 * Connection state constants
 */
export const CONNECTION_STATES = {
  CONNECTING: 'connecting',
  CONNECTED: 'connected',
  DISCONNECTED: 'disconnected',
}

/**
 * WebSocket slice - manages ALL WebSocket connection, messaging, and state
 * Consolidates connection management, message sending/receiving, and typing indicators
 */
export const createWebSocketSlice = (set, get) => {
  // Private state not exposed to components
  let wsInstance = null
  let reconnectTimeout = null
  let reconnectAttempts = 0
  const maxReconnectAttempts = 5

  // Message handlers
  const handleThinkingStep = (data) => {
    const { addMessage, setCurrentThinking, setIsTyping } = get()

    if (data.complete) {
      addMessage(
        createMessage(MESSAGE_TYPES.THINKING_STEP, '', {
          data: { ...data },
        })
      )
      setCurrentThinking(null)
    } else {
      setIsTyping(true)
      setCurrentThinking({ ...data })
    }
  }

  const handleFinalResponse = (data) => {
    const { addMessage, setIsTyping, setCurrentThinking, pageContext } = get()

    setIsTyping(false)
    setCurrentThinking(null)
    addMessage(
      createMessage(MESSAGE_TYPES.ASSISTANT, data.response, {
        tools_used: data.tools_used,
        visualizations: data.visualizations || [],
        timestamp: data.timestamp,
        pageContext: pageContext,
      })
    )
  }

  const handleError = (data) => {
    const { addMessage, setIsTyping, setCurrentThinking } = get()

    setIsTyping(false)
    setCurrentThinking(null)
    addMessage(
      createMessage(MESSAGE_TYPES.ERROR, data.error, {
        timestamp: data.timestamp,
      })
    )
    set({ error: data.error })
  }

  const handleWebSocketMessage = (message) => {
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
  }

  return {
    // State
    connectionState: CONNECTION_STATES.DISCONNECTED,
    isTyping: false,
    error: null,

    // Simple state setters
    setConnectionState: (state) => {
      set({ connectionState: state })
    },

    setIsTyping: (isTyping) => {
      set({ isTyping })
    },

    setError: (error) => {
      set({ error })
    },

    // WebSocket connection management
    connectWebSocket: () => {
      if (wsInstance?.readyState === WebSocket.OPEN) {
        return
      }

      const { setConnectionState, setError, setIsTyping, setCurrentThinking } =
        get()

      try {
        const wsUrl = getChatWebSocketUrl()
        console.log('Connecting to WebSocket:', wsUrl)

        wsInstance = new WebSocket(wsUrl)
        setConnectionState(CONNECTION_STATES.CONNECTING)
        setError(null)

        wsInstance.onopen = () => {
          console.log('WebSocket connected')
          setConnectionState(CONNECTION_STATES.CONNECTED)
          reconnectAttempts = 0
          setError(null)

          // Fetch prompts whenever we connect/reconnect
          const { fetchPrompts } = get()
          if (fetchPrompts) {
            fetchPrompts()
          }
        }

        wsInstance.onmessage = (event) => {
          const message = parseWebSocketMessage(event.data)
          if (!message) return
          handleWebSocketMessage(message)
        }

        wsInstance.onclose = (event) => {
          console.log('WebSocket closed:', event.code, event.reason)
          setConnectionState(CONNECTION_STATES.DISCONNECTED)
          setIsTyping(false)
          setCurrentThinking(null)

          // Auto-reconnect if not a clean close
          if (event.code !== 1000 && reconnectAttempts < maxReconnectAttempts) {
            const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 30000)
            console.log(
              `Reconnecting in ${delay}ms (attempt ${reconnectAttempts + 1})`
            )
            reconnectTimeout = setTimeout(() => {
              reconnectAttempts++
              get().connectWebSocket()
            }, delay)
          }
        }

        wsInstance.onerror = (error) => {
          console.error('WebSocket error:', error)
          setError('Connection error occurred')
        }
      } catch (err) {
        console.error('Failed to create WebSocket connection:', err)
        setError('Failed to connect to chat service')
        setConnectionState(CONNECTION_STATES.DISCONNECTED)
      }
    },

    disconnectWebSocket: () => {
      if (reconnectTimeout) {
        clearTimeout(reconnectTimeout)
        reconnectTimeout = null
      }

      if (wsInstance) {
        get().setConnectionState(CONNECTION_STATES.DISCONNECTED)
        wsInstance.close(1000, 'User disconnected')
        wsInstance = null
      }
    },

    // Stop current generation by disconnecting and reconnecting
    stopGeneration: () => {
      const {
        disconnectWebSocket,
        connectWebSocket,
        setIsTyping,
        setCurrentThinking,
      } = get()

      setIsTyping(false)
      setCurrentThinking(null)
      disconnectWebSocket()
      setTimeout(() => {
        connectWebSocket()
      }, 100)
    },

    // Send message through WebSocket
    sendMessage: (content) => {
      const {
        connectionState,
        getActiveSession,
        addMessage,
        setError,
        setIsTyping,
        pageContext,
        settings,
      } = get()

      if (connectionState !== CONNECTION_STATES.CONNECTED) {
        setError('Not connected to chat service')
        return false
      }

      try {
        addMessage(
          createMessage(MESSAGE_TYPES.USER, content, {
            pageContext: pageContext,
          })
        )

        // Get messages from active session for chat history
        const activeSession = getActiveSession()
        const messages = activeSession?.messages || []

        const payload = {
          message: content,
          chat_history: formatChatHistoryForAPI(messages),
          show_thinking: true,
          persona: settings.persona || 'default',
          page_context: pageContext,
        }

        wsInstance.send(JSON.stringify(payload))

        setError(null)
        setIsTyping(true)
        return true
      } catch (err) {
        console.error('Failed to send message:', err)
        setError('Failed to send message')
        return false
      }
    },
  }
}
