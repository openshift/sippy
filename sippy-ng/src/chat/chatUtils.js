// Chat utility functions and constants

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
  const baseUrl = process.env.REACT_APP_CHAT_API_URL || 'http://localhost:8000'
  const host = new URL(baseUrl).host

  return `${protocol}//${host}/chat/stream`
}

// Get REST API URL for chat
export function getChatRestUrl() {
  const baseUrl = process.env.REACT_APP_CHAT_API_URL || 'http://localhost:8000'
  return `${baseUrl}/chat`
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

// Default chat settings
export const DEFAULT_CHAT_SETTINGS = {
  showThinking: true,
  autoScroll: true,
  maxHistory: 100,
  retryFailedMessages: true,
  streamingMode: true,
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

// Extract tool names from thinking steps
export function extractToolsUsed(messages) {
  const tools = new Set()

  messages.forEach((msg) => {
    if (msg.type === MESSAGE_TYPES.THINKING_STEP && msg.data?.action) {
      tools.add(msg.data.action)
    }
  })

  return Array.from(tools)
}

// Check if message is from today
export function isMessageFromToday(timestamp) {
  const messageDate = new Date(timestamp)
  const today = new Date()

  return messageDate.toDateString() === today.toDateString()
}

// Group messages by date
export function groupMessagesByDate(messages) {
  const groups = {}

  messages.forEach((msg) => {
    const date = new Date(msg.timestamp).toDateString()
    if (!groups[date]) {
      groups[date] = []
    }
    groups[date].push(msg)
  })

  return groups
}

// Sanitize message content for display
export function sanitizeMessageContent(content) {
  if (typeof content !== 'string') return ''

  // Basic HTML escaping
  return content
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#x27;')
}
