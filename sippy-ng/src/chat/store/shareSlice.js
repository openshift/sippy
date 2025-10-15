import { createMessage, MESSAGE_TYPES } from '../chatUtils'
import { relativeTime } from '../../helpers'
import { SESSION_TYPES } from './sessionSlice'

/**
 * Share slice - manages conversation sharing functionality
 */
export const createShareSlice = (set, get) => ({
  // State
  shareLoading: false,
  shareDialogOpen: false,
  sharedUrl: '',
  shareSnackbar: {
    open: false,
    message: '',
    severity: 'success',
  },
  loadingShared: false,

  // Load shared conversation from API
  loadSharedConversationFromAPI: (conversationId) => {
    const { sessions, loadSharedConversation } = get()

    // Don't fetch if we've already loaded this conversation - just switch to it
    const existingSession = sessions.find((s) => s.sharedId === conversationId)
    if (existingSession) {
      set({ activeSessionId: existingSession.id })
      return
    }

    const abortController = new AbortController()

    set({ loadingShared: true })

    fetch(
      `${process.env.REACT_APP_API_URL}/api/chat/conversations/${conversationId}`,
      { signal: abortController.signal }
    )
      .then((response) => {
        if (!response.ok) {
          return response.json().then(
            (errorData) => {
              throw new Error(
                errorData.message || 'Failed to load shared conversation'
              )
            },
            () => {
              throw new Error(
                response.statusText || 'Failed to load shared conversation'
              )
            }
          )
        }
        return response.json()
      })
      .then((data) => {
        // Load shared messages
        const loadedMessages = data.messages.map((msg, idx) => ({
          ...msg,
          id: msg.id || `loaded_${idx}`,
        }))

        // Add a system message to mark this shared conversation
        const systemMessage = createMessage(
          MESSAGE_TYPES.SYSTEM,
          `Shared by ${data.user} ${relativeTime(
            new Date(data.created_at),
            new Date()
          )}`,
          {
            id: 'system_' + conversationId,
            conversationId: conversationId,
            timestamp: data.created_at,
          }
        )

        const allMessages = [...loadedMessages, systemMessage]

        // Load into session management (this will set messages in the session)
        loadSharedConversation(conversationId, allMessages, {
          createdAt: data.created_at,
          parentId: data.parent_id,
          sharedBy: data.user,
        })

        // Set the shared URL so clicking share again doesn't create a duplicate
        const url = `${window.location.origin}/sippy-ng/chat/${conversationId}`
        set({
          sharedUrl: url,
          loadingShared: false,
        })
      })
      .catch((err) => {
        // Don't show error for aborted requests
        if (err.name === 'AbortError') {
          return
        }
        console.error('Error loading shared conversation:', err)
        set({
          shareSnackbar: {
            open: true,
            message: err.message,
            severity: 'error',
          },
          loadingShared: false,
        })
      })
  },

  // Share the current conversation
  shareConversation: (pageContext, mode = 'fullPage') => {
    const {
      settings,
      getActiveSession,
      activeSessionId,
      updateSessionMetadata,
    } = get()

    const activeSession = getActiveSession()
    const messages = activeSession?.messages || []

    // Filter out system messages for sharing
    const filteredMessages = messages.filter(
      (msg) => msg.type !== MESSAGE_TYPES.SYSTEM
    )

    if (filteredMessages.length === 0) {
      set({
        shareSnackbar: {
          open: true,
          message: 'No messages to share',
          severity: 'warning',
        },
      })
      return
    }

    // If session already has a sharedId, it's already been shared - just show the dialog
    if (activeSession?.sharedId) {
      const url = `${window.location.origin}/sippy-ng/chat/${activeSession.sharedId}`
      set({
        sharedUrl: url,
        shareDialogOpen: true,
      })
      return
    }

    set({ shareLoading: true })

    // Format messages for API
    const messagesToShare = filteredMessages.map((msg) => ({
      type: msg.type,
      content: msg.content,
      timestamp: msg.timestamp,
      ...(msg.data && { data: msg.data }),
      ...(msg.pageContext && { pageContext: msg.pageContext }),
      ...(msg.conversationId && { conversationId: msg.conversationId }),
    }))

    // Prepare metadata
    const metadata = {
      persona: settings.persona,
      pageContext: pageContext,
      sharedAt: new Date().toISOString(),
    }

    // If we're sharing a forked conversation, include the parent ID
    const payload = {
      messages: messagesToShare,
      metadata: metadata,
    }

    if (activeSession?.sharedId || activeSession?.parentId) {
      payload.parent_id = activeSession.sharedId || activeSession.parentId
    }

    fetch(process.env.REACT_APP_API_URL + '/api/chat/conversations', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    })
      .then((response) => {
        if (!response.ok) {
          return response.json().then(
            (errorData) => {
              throw new Error(
                errorData.message || 'Failed to share conversation'
              )
            },
            () => {
              throw new Error(
                response.statusText || 'Failed to share conversation'
              )
            }
          )
        }
        return response.json()
      })
      .then((data) => {
        // Construct the shareable URL
        const url = `${window.location.origin}/sippy-ng/chat/${data.id}`
        set({
          sharedUrl: url,
          shareDialogOpen: true,
        })

        // Update session metadata
        if (activeSessionId) {
          updateSessionMetadata(activeSessionId, {
            sharedId: data.id,
            type: SESSION_TYPES.SHARED_BY_ME,
          })
        }

        // Update browser URL (only in fullPage mode)
        if (mode === 'fullPage') {
          window.history.pushState(null, '', `/sippy-ng/chat/${data.id}`)
        }

        // Copy to clipboard
        navigator.clipboard.writeText(url).catch((err) => {
          console.warn('Failed to copy to clipboard:', err)
        })
      })
      .catch((err) => {
        console.error('Error sharing conversation:', err)
        set({
          shareSnackbar: {
            open: true,
            message: err.message || 'Failed to share conversation',
            severity: 'error',
          },
        })
      })
      .finally(() => {
        set({ shareLoading: false })
      })
  },

  // Clear shared URL
  clearSharedUrl: () => {
    set({ sharedUrl: '' })
  },

  // UI actions
  setShareDialogOpen: (open) => {
    set({ shareDialogOpen: open })
  },

  setShareSnackbar: (snackbar) => {
    set({ shareSnackbar: snackbar })
  },

  closeShareSnackbar: () => {
    set((state) => ({
      shareSnackbar: { ...state.shareSnackbar, open: false },
    }))
  },

  copyToClipboard: () => {
    const { sharedUrl } = get()
    navigator.clipboard
      .writeText(sharedUrl)
      .then(() => {
        set({
          shareSnackbar: {
            open: true,
            message: 'Link copied to clipboard!',
            severity: 'success',
          },
        })
      })
      .catch((err) => {
        set({
          shareSnackbar: {
            open: true,
            message: 'Failed to copy to clipboard',
            severity: 'error',
          },
        })
      })
  },
})
