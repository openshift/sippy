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
  const [isTyping, setIsTyping] = useState(false)
  const [messageQueue, setMessageQueue] = useState([])

  const wsRef = useRef(null)
  const reconnectTimeoutRef = useRef(null)
  const reconnectAttemptsRef = useRef(0)
  const currentIterationRef = useRef(0)
  const pageContextRef = useRef(pageContext)
  const processingQueueRef = useRef(false)
  const sendMessageNowRef = useRef(null)
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

    // Process next queued message directly (no effect needed)
    setTimeout(() => {
      if (processingQueueRef.current) {
        console.log('[Queue] Already processing, skipping direct call')
        return
      }

      setMessageQueue((queue) => {
        if (queue.length === 0) {
          return queue
        }

        processingQueueRef.current = true
        const [next, ...rest] = queue
        console.log(
          '[Queue] Direct processing:',
          next,
          '| Remaining:',
          rest.length
        )

        setTimeout(() => {
          if (sendMessageNowRef.current) {
            sendMessageNowRef.current(next)
          }
          setTimeout(() => {
            processingQueueRef.current = false
          }, 1000)
        }, 300)

        return rest
      })
    }, 400)
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

  // Actually send message to WebSocket (internal function)
  const sendMessageNow = useCallback(
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

  // Keep ref updated for queue processing
  useEffect(() => {
    sendMessageNowRef.current = sendMessageNow
  }, [sendMessageNow])

  // Send message (public API) - queues if currently typing
  const sendMessage = useCallback(
    (content) => {
      if (connectionState !== WEBSOCKET_STATES.OPEN) {
        setError('Not connected to chat service')
        return false
      }

      // If currently typing, add to queue
      if (isTyping) {
        console.log('Queueing message:', content)
        setMessageQueue((prev) => [...prev, content])
        return true
      }

      // Send immediately if not typing
      return sendMessageNow(content)
    },
    [connectionState, isTyping, sendMessageNow]
  )

  // Clear chat history
  const clearMessages = useCallback(() => {
    setMessages([])
    setCurrentThinking(null)
    setError(null)
    setIsTyping(false)
    setMessageQueue([])
    processingQueueRef.current = false
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

  // Stop current generation
  const stopGeneration = useCallback(() => {
    console.log('Stopping generation...')
    setIsTyping(false)
    setCurrentThinking(null)
    setMessageQueue([])
    processingQueueRef.current = false

    const stopMessage = createMessage(
      MESSAGE_TYPES.SYSTEM,
      'Generation stopped by user',
      { variant: 'info' }
    )
    setMessages((prev) => [...prev, stopMessage])

    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.close(1000, 'User stopped generation')
      wsRef.current = null
      setTimeout(() => connect(), 100)
    }
  }, [connect])

  // Send queued message at index (stop current, send this)
  const sendMessageAtIndex = useCallback(
    (index) => {
      setMessageQueue((queue) => {
        if (index >= queue.length) return queue

        const messageToSend = queue[index]
        const newQueue = queue.filter((_, i) => i !== index)

        setIsTyping(false)
        setCurrentThinking(null)
        processingQueueRef.current = false

        const switchMessage = createMessage(
          MESSAGE_TYPES.SYSTEM,
          'Switching to queued message...',
          { variant: 'info' }
        )
        setMessages((prev) => [...prev, switchMessage])

        if (wsRef.current?.readyState === WebSocket.OPEN) {
          const ws = wsRef.current
          const onClose = () => {
            ws.removeEventListener('close', onClose)
            setTimeout(() => {
              connect()
              setTimeout(() => {
                if (
                  wsRef.current?.readyState === WebSocket.OPEN &&
                  sendMessageNowRef.current
                ) {
                  sendMessageNowRef.current(messageToSend)
                }
              }, 500)
            }, 200)
          }
          ws.addEventListener('close', onClose)
          ws.close(1000, 'Sending queued message')
          wsRef.current = null
        }

        return newQueue
      })
    },
    [connect]
  )

  // Delete queued message at index
  const deleteMessageAtIndex = useCallback((index) => {
    setMessageQueue((queue) => queue.filter((_, i) => i !== index))
  }, [])

  // Clear all queued messages
  const clearQueue = useCallback(() => {
    setMessageQueue([])
    processingQueueRef.current = false
  }, [])

  return {
    messages,
    connectionState,
    currentThinking,
    error,
    isTyping,
    messageQueue,
    sendMessage,
    clearMessages,
    stopGeneration,
    sendMessageAtIndex,
    deleteMessageAtIndex,
    clearQueue,
    connect,
    disconnect,
    isConnected: connectionState === WEBSOCKET_STATES.OPEN,
  }
}
