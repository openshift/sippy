const MAX_SESSIONS = 50

// Session types
export const SESSION_TYPES = {
  OWN: 'own',
  SHARED: 'shared',
  SHARED_BY_ME: 'shared_by_me',
  FORKED: 'forked',
}

// Generate a unique session ID
function generateSessionId() {
  return `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
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

/**
 * Session slice - manages multiple chat sessions
 */
export const createSessionSlice = (set, get) => ({
  // State
  sessions: [],
  activeSessionId: null,
  currentThinking: null,

  // Initialize sessions from IndexedDB (called on app mount)
  initializeSessions: () => {
    const state = get()
    if (state.sessions.length === 0) {
      // Create initial session if none exist
      const newSession = createNewSession()
      set({
        sessions: [newSession],
        activeSessionId: newSession.id,
      })
    }
  },

  // Get the currently active session
  getActiveSession: () => {
    const { sessions, activeSessionId } = get()
    return sessions.find((s) => s.id === activeSessionId) || null
  },

  // Create a new session
  createSession: (type = SESSION_TYPES.OWN, options = {}) => {
    const newSession = createNewSession(type, options)
    set((state) => {
      const updated = [newSession, ...state.sessions]
      return {
        sessions:
          updated.length > MAX_SESSIONS
            ? updated.slice(0, MAX_SESSIONS)
            : updated,
        activeSessionId: newSession.id,
      }
    })
    return newSession
  },

  // Switch to a different session
  switchSession: (sessionId) => {
    set({ activeSessionId: sessionId })
  },

  // Start a new session (find empty or create new)
  // If initialMessage is provided, waits for websocket connection and sends it
  startNewSession: (initialMessage = null) => {
    const {
      sessions,
      activeSessionId,
      switchSession,
      createSession,
      setCurrentThinking,
      setError,
      setIsTyping,
      connectionState,
      sendMessage,
    } = get()

    // Try to find an existing empty session
    const emptySession = sessions.find(
      (s) =>
        s.type === SESSION_TYPES.OWN &&
        (!s.messages || s.messages.length === 0) &&
        !s.sharedId
    )

    // Switch to empty session or create new one
    if (emptySession && emptySession.id !== activeSessionId) {
      switchSession(emptySession.id)
    } else if (!emptySession) {
      createSession()
    }

    // Reset WebSocket state for the new session
    setCurrentThinking(null)
    setError(null)
    setIsTyping(false)

    // If an initial message is provided, wait for connection and send it
    if (initialMessage) {
      const maxAttempts = 50 // 5 seconds max (50 * 100ms)
      let attempts = 0

      const waitForConnection = () => {
        const store = get()
        if (store.connectionState === 'connected') {
          sendMessage(initialMessage)
        } else if (attempts < maxAttempts) {
          attempts++
          setTimeout(waitForConnection, 100)
        } else {
          console.error('Failed to connect to websocket after 5 seconds')
          setError('Failed to connect to chat service')
        }
      }

      waitForConnection()
    }
  },

  // Delete a session
  deleteSession: (sessionId) => {
    const { activeSessionId } = get()
    const isActiveSession = sessionId === activeSessionId

    set((state) => {
      const filtered = state.sessions.filter((s) => s.id !== sessionId)

      // If we deleted the active session, find or create an empty "New Chat"
      if (isActiveSession) {
        const emptySession = filtered.find(
          (s) =>
            s.type === SESSION_TYPES.OWN &&
            (!s.messages || s.messages.length === 0) &&
            !s.sharedId
        )

        if (emptySession) {
          return {
            sessions: filtered,
            activeSessionId: emptySession.id,
          }
        } else {
          const newSession = createNewSession()
          return {
            sessions: [newSession, ...filtered],
            activeSessionId: newSession.id,
          }
        }
      }

      return { sessions: filtered }
    })

    return isActiveSession
  },

  // Update session metadata
  updateSessionMetadata: (sessionId, updates) => {
    set((state) => ({
      sessions: state.sessions.map((session) =>
        session.id === sessionId
          ? {
              ...session,
              ...updates,
              updatedAt: new Date().toISOString(),
            }
          : session
      ),
    }))
  },

  // Fork the current session (when user modifies a shared chat)
  forkActiveSession: () => {
    const currentSession = get().getActiveSession()
    if (!currentSession) return null

    // Only fork if it's a shared session
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
      sharedId: null, // Clear sharedId - this is a new fork, not a shared conversation
      sharedBy: undefined, // Remove sharedBy since this is our fork
      messages: [...(currentSession.messages || [])],
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }

    set((state) => ({
      sessions: [forkedSession, ...state.sessions],
      activeSessionId: forkedSession.id,
    }))

    return forkedSession
  },

  // Load a shared conversation (from URL)
  loadSharedConversation: (conversationId, messages, metadata) => {
    const { sessions } = get()

    // Check if this shared conversation already exists
    const existingSession = sessions.find((s) => s.sharedId === conversationId)

    if (existingSession) {
      set({ activeSessionId: existingSession.id })
      return existingSession
    }

    // Create new shared session
    const sharedSession = {
      id: conversationId,
      type: SESSION_TYPES.SHARED,
      sharedId: conversationId,
      parentId: metadata?.parentId || null,
      sharedBy: metadata?.sharedBy || null,
      messages,
      createdAt: metadata?.createdAt || new Date().toISOString(),
      updatedAt: metadata?.createdAt || new Date().toISOString(),
    }

    set((state) => {
      const updated = [sharedSession, ...state.sessions]
      return {
        sessions:
          updated.length > MAX_SESSIONS
            ? updated.slice(0, MAX_SESSIONS)
            : updated,
        activeSessionId: sharedSession.id,
      }
    })

    return sharedSession
  },

  // Clear all sessions (for testing or reset)
  clearAllSessions: () => {
    const newSession = createNewSession()
    set({
      sessions: [newSession],
      activeSessionId: newSession.id,
    })
  },

  // Clear sessions older than specified days
  clearOldSessions: (days) => {
    const cutoffDate = new Date()
    cutoffDate.setDate(cutoffDate.getDate() - days)

    const { sessions, activeSessionId } = get()

    // Helper to get the timestamp shown to users (last message or updatedAt)
    const getSessionTimestamp = (session) => {
      const messages = session.messages || []
      return messages.length > 0
        ? messages[messages.length - 1].timestamp
        : session.updatedAt
    }

    // Filter out sessions older than cutoff based on the timestamp users see
    const filteredSessions = sessions.filter((session) => {
      const sessionDate = new Date(getSessionTimestamp(session))
      return sessionDate >= cutoffDate
    })

    // If active session was removed, select the first remaining session or create new
    let newActiveSessionId = activeSessionId
    if (
      activeSessionId &&
      !filteredSessions.find((s) => s.id === activeSessionId)
    ) {
      if (filteredSessions.length > 0) {
        newActiveSessionId = filteredSessions[0].id
      } else {
        const newSession = createNewSession()
        filteredSessions.push(newSession)
        newActiveSessionId = newSession.id
      }
    }

    // If no sessions remain, create a new one
    if (filteredSessions.length === 0) {
      const newSession = createNewSession()
      filteredSessions.push(newSession)
      newActiveSessionId = newSession.id
    }

    set({
      sessions: filteredSessions,
      activeSessionId: newActiveSessionId,
    })

    return sessions.length - filteredSessions.length // Return count of cleared sessions
  },

  // Message operations - operate on messages within the active session
  // Add a message to the active session
  addMessage: (message) => {
    const { activeSessionId, sessions } = get()
    if (!activeSessionId) {
      console.warn('Cannot add message: no active session')
      return
    }

    set((state) => ({
      sessions: state.sessions.map((session) =>
        session.id === activeSessionId
          ? {
              ...session,
              messages: [...(session.messages || []), message],
              updatedAt: new Date().toISOString(),
            }
          : session
      ),
    }))
  },

  // Clear messages from the active session
  clearMessages: () => {
    const { activeSessionId } = get()
    if (!activeSessionId) {
      console.warn('Cannot clear messages: no active session')
      return
    }

    set((state) => ({
      sessions: state.sessions.map((session) =>
        session.id === activeSessionId
          ? {
              ...session,
              messages: [],
              updatedAt: new Date().toISOString(),
            }
          : session
      ),
      currentThinking: null,
    }))
  },

  // Set current thinking state (ephemeral UI state)
  setCurrentThinking: (thinking) => {
    set({ currentThinking: thinking })
  },
})
