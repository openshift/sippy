import { useCallback, useEffect, useState } from 'react'

const STORAGE_KEY = 'sippyChatSessions'
const MAX_SESSIONS = 50

// Session types
export const SESSION_TYPES = {
  OWN: 'own', // User's own chat
  SHARED: 'shared', // Loaded from shared URL, not modified
  SHARED_BY_ME: 'shared_by_me', // User shared this conversation
  FORKED: 'forked', // Shared chat that was modified
}

/**
 * Hook to manage multiple chat sessions in localStorage
 * Each session contains messages, metadata, and type information
 */
export function useChatSessions() {
  const [activeSessionId, setActiveSessionId] = useState(null)
  const [sessions, setSessions] = useState([])

  // Load sessions from localStorage on mount
  useEffect(() => {
    const loadSessions = () => {
      try {
        const stored = localStorage.getItem(STORAGE_KEY)
        if (stored) {
          const data = JSON.parse(stored)
          setSessions(data.sessions || [])

          // If there's an active session, use it; otherwise create a new one
          if (
            data.activeSessionId &&
            data.sessions?.find((s) => s.id === data.activeSessionId)
          ) {
            setActiveSessionId(data.activeSessionId)
          } else if (data.sessions?.length > 0) {
            setActiveSessionId(data.sessions[0].id)
          } else {
            // Create initial session if none exist
            const newSession = createNewSession()
            setSessions([newSession])
            setActiveSessionId(newSession.id)
          }
        } else {
          // First time - create a new session
          const newSession = createNewSession()
          setSessions([newSession])
          setActiveSessionId(newSession.id)
        }
      } catch (error) {
        console.error('Error loading chat sessions:', error)
        // On error, start fresh
        const newSession = createNewSession()
        setSessions([newSession])
        setActiveSessionId(newSession.id)
      }
    }

    loadSessions()
  }, [])

  // Save sessions to localStorage whenever they change
  useEffect(() => {
    if (sessions.length > 0 && activeSessionId) {
      try {
        const data = {
          activeSessionId,
          sessions,
        }
        localStorage.setItem(STORAGE_KEY, JSON.stringify(data))
      } catch (error) {
        console.error('Error saving chat sessions:', error)
      }
    }
  }, [sessions, activeSessionId])

  // Get the currently active session
  const getActiveSession = useCallback(() => {
    return sessions.find((s) => s.id === activeSessionId) || null
  }, [sessions, activeSessionId])

  // Create a new session
  const createSession = useCallback(
    (type = SESSION_TYPES.OWN, options = {}) => {
      const newSession = createNewSession(type, options)

      setSessions((prev) => {
        // Limit to MAX_SESSIONS
        const updated = [newSession, ...prev]
        if (updated.length > MAX_SESSIONS) {
          return updated.slice(0, MAX_SESSIONS)
        }
        return updated
      })

      setActiveSessionId(newSession.id)
      return newSession
    },
    []
  )

  // Switch to a different session
  const switchSession = useCallback((sessionId) => {
    setActiveSessionId(sessionId)
  }, [])

  // Delete a session
  const deleteSession = useCallback(
    (sessionId) => {
      const isActiveSession = sessionId === activeSessionId

      setSessions((prev) => {
        const filtered = prev.filter((s) => s.id !== sessionId)

        // If we deleted the active session, find or create an empty "New Chat"
        if (isActiveSession) {
          // Look for an existing empty "New Chat" session
          const emptySession = filtered.find(
            (s) =>
              s.type === SESSION_TYPES.OWN &&
              (!s.messages || s.messages.length === 0) &&
              !s.sharedId
          )

          if (emptySession) {
            // Switch to the existing empty session
            setActiveSessionId(emptySession.id)
            return filtered
          } else {
            // No empty session exists, create a new one
            const newSession = createNewSession()
            setActiveSessionId(newSession.id)
            return [newSession, ...filtered]
          }
        }

        return filtered
      })

      return isActiveSession // Return true if we deleted the active session
    },
    [activeSessionId]
  )

  // Update messages in the active session
  const updateSessionMessages = useCallback(
    (messages) => {
      setSessions((prev) =>
        prev.map((session) =>
          session.id === activeSessionId
            ? {
                ...session,
                messages,
                updatedAt: new Date().toISOString(),
              }
            : session
        )
      )
    },
    [activeSessionId]
  )

  // Update session metadata (e.g., after sharing)
  const updateSessionMetadata = useCallback((sessionId, updates) => {
    setSessions((prev) =>
      prev.map((session) =>
        session.id === sessionId
          ? {
              ...session,
              ...updates,
              updatedAt: new Date().toISOString(),
            }
          : session
      )
    )
  }, [])

  // Fork the current session (when user modifies a shared chat)
  const forkActiveSession = useCallback(() => {
    const currentSession = getActiveSession()
    if (!currentSession) return null

    // Only fork if it's a shared session (either shared with you or shared by you)
    if (
      currentSession.type !== SESSION_TYPES.SHARED &&
      currentSession.type !== SESSION_TYPES.SHARED_BY_ME
    ) {
      return currentSession
    }

    // Create a forked session with a copy of the current messages
    const forkedSession = {
      ...currentSession,
      id: generateSessionId(),
      type: SESSION_TYPES.FORKED,
      parentId: currentSession.sharedId || currentSession.id,
      messages: [...(currentSession.messages || [])], // Copy messages array
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }

    setSessions((prev) => {
      // Add the forked session to the list (don't replace the original)
      // Put the fork at the beginning since it's now active
      return [forkedSession, ...prev]
    })

    setActiveSessionId(forkedSession.id)
    return forkedSession
  }, [getActiveSession, activeSessionId])

  // Load a shared conversation (from URL)
  const loadSharedConversation = useCallback(
    (conversationId, messages, metadata) => {
      // Check if this shared conversation already exists
      const existingSession = sessions.find(
        (s) => s.sharedId === conversationId
      )

      if (existingSession) {
        // Just switch to it
        setActiveSessionId(existingSession.id)
        return existingSession
      }

      // Create new shared session
      const sharedSession = {
        id: conversationId, // Use the UUID as the session ID
        type: SESSION_TYPES.SHARED,
        sharedId: conversationId,
        parentId: metadata?.parentId || null,
        messages,
        createdAt: metadata?.createdAt || new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      }

      setSessions((prev) => {
        const updated = [sharedSession, ...prev]
        if (updated.length > MAX_SESSIONS) {
          return updated.slice(0, MAX_SESSIONS)
        }
        return updated
      })

      setActiveSessionId(sharedSession.id)
      return sharedSession
    },
    [sessions]
  )

  // Clear all sessions (for testing or reset)
  const clearAllSessions = useCallback(() => {
    const newSession = createNewSession()
    setSessions([newSession])
    setActiveSessionId(newSession.id)
    localStorage.removeItem(STORAGE_KEY)
  }, [])

  return {
    // State
    sessions,
    activeSessionId,
    activeSession: getActiveSession(),

    // Actions
    createSession,
    switchSession,
    deleteSession,
    updateSessionMessages,
    updateSessionMetadata,
    forkActiveSession,
    loadSharedConversation,
    clearAllSessions,
  }
}

// Helper to create a new session object
function createNewSession(type = SESSION_TYPES.OWN, options = {}) {
  return {
    id: options.id || generateSessionId(),
    type,
    sharedId: options.sharedId || null,
    parentId: options.parentId || null,
    messages: options.messages || [],
    createdAt: options.createdAt || new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  }
}

// Generate a unique session ID
function generateSessionId() {
  return `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

// Get a display title for a session (first message preview)
export function getSessionTitle(session) {
  if (!session || !session.messages || session.messages.length === 0) {
    return 'New Chat'
  }

  // Find the first user message
  const firstUserMessage = session.messages.find((msg) => msg.type === 'user')
  if (firstUserMessage && firstUserMessage.content) {
    // Truncate to 40 characters
    const content = firstUserMessage.content.trim()
    return content.length > 40 ? content.substring(0, 40) + '...' : content
  }

  return 'New Chat'
}

// Get icon name for session type (Material UI icon)
export function getSessionIconName(sessionType) {
  switch (sessionType) {
    case SESSION_TYPES.OWN:
      return 'Chat'
    case SESSION_TYPES.SHARED:
      return 'Link'
    case SESSION_TYPES.SHARED_BY_ME:
      return 'Share'
    case SESSION_TYPES.FORKED:
      return 'CallSplit'
    default:
      return 'Chat'
  }
}

// Format session timestamp for display
export function formatSessionTime(timestamp) {
  if (!timestamp) return ''

  const date = new Date(timestamp)
  const now = new Date()
  const diffMs = now - date
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMins / 60)
  const diffDays = Math.floor(diffHours / 24)

  if (diffMins < 1) return 'Just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays < 7) return `${diffDays}d ago`

  // For older dates, show the date
  return date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
  })
}
