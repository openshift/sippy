// Chat utility functions and constants

/**
 * Convert kebab-case or snake_case strings to Title Case
 * Examples:
 *   humanize('hello-world') => 'Hello World'
 *   humanize('hello_world') => 'Hello World'
 *   humanize('component-readiness') => 'Component Readiness'
 */
export function humanize(str) {
  if (!str) return ''

  return str
    .split(/[-_]/)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

export const MESSAGE_TYPES = {
  USER: 'user',
  ASSISTANT: 'assistant',
  THINKING_STEP: 'thinking_step',
  FINAL_RESPONSE: 'final_response',
  ERROR: 'error',
  SYSTEM: 'system',
}

export const WEBSOCKET_STATES = {
  CONNECTING: 0,
  OPEN: 1,
  CLOSING: 2,
  CLOSED: 3,
}

// Format timestamp to second precision with relative time tooltip
export function formatChatTimestamp(timestamp) {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  return {
    main: date.toISOString().replace(/\.\d{3}Z$/, 'Z'),
    relative: getRelativeTime(date),
  }
}

// Get relative time for tooltips
function getRelativeTime(date) {
  const now = new Date()
  const diffMs = now - date
  const diffSecs = Math.floor(diffMs / 1000)
  const diffMins = Math.floor(diffSecs / 60)
  const diffHours = Math.floor(diffMins / 60)
  const diffDays = Math.floor(diffHours / 24)

  if (diffSecs < 60) return `${diffSecs} seconds ago`
  if (diffMins < 60) return `${diffMins} minutes ago`
  if (diffHours < 24) return `${diffHours} hours ago`
  return `${diffDays} days ago`
}

// Get WebSocket URL based on current environment
export function getChatWebSocketUrl() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const baseUrl = process.env.REACT_APP_CHAT_API_URL || '/api/chat'

  let url
  if (baseUrl.startsWith('/')) {
    url = new URL(baseUrl, window.location.origin)
  } else {
    url = new URL(baseUrl)
    url.protocol = protocol
  }

  url.pathname = url.pathname.replace(/\/$/, '') + '/stream'
  return url.toString()
}

// Create a new message object
export function createMessage(type, content, options = {}) {
  return {
    id: generateMessageId(),
    type,
    content,
    timestamp: new Date().toISOString(),
    ...options,
  }
}

// Generate unique message ID
function generateMessageId() {
  return `msg_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

// Convert chat history to API format
export function formatChatHistoryForAPI(messages) {
  return messages
    .filter(
      (msg) =>
        msg.type === MESSAGE_TYPES.USER || msg.type === MESSAGE_TYPES.ASSISTANT
    )
    .map((msg) => ({
      role: msg.type === MESSAGE_TYPES.USER ? 'user' : 'assistant',
      content: msg.content,
      timestamp: msg.timestamp,
      page_context: msg.pageContext || null,
    }))
}

// Parse WebSocket message
export function parseWebSocketMessage(data) {
  try {
    return JSON.parse(data)
  } catch (error) {
    console.error('Failed to parse WebSocket message:', error)
    return null
  }
}

// Validate message content
export function validateMessage(content) {
  if (!content || typeof content !== 'string') {
    return { valid: false, error: 'Message content is required' }
  }

  if (content.trim().length === 0) {
    return { valid: false, error: 'Message cannot be empty' }
  }

  if (content.length > 10000) {
    return {
      valid: false,
      error: 'Message is too long (max 10,000 characters)',
    }
  }

  return { valid: true }
}
