import { MESSAGE_TYPES } from './chatUtils'
import { SESSION_TYPES } from './store/sessionSlice'
import { useEffect, useRef } from 'react'
import { useSessionActions } from './store/useChatStore'

/**
 * Custom hook for managing chat scroll behavior
 *
 * Scroll behavior rules:
 * 1. User sends message → Always scroll to bottom (they're engaged)
 * 2. Assistant/system messages → Only scroll if user is "following" (at bottom or just sent message)
 * 3. User manually scrolls up → Stop auto-scrolling (they're reading history)
 * 4. Session switch → Restore saved scroll position if available
 *    - Shared conversations (first load): Start at top
 *    - Your conversations (first load): Start at bottom
 * 5. Scroll position is saved per session and persisted
 */
export function useScrollManagement(
  activeSessionId,
  activeSession,
  messages,
  settings
) {
  const messagesEndRef = useRef(null)
  const messagesListRef = useRef(null)
  const lastMessageRef = useRef(null)
  const prevSessionIdRef = useRef(activeSessionId)
  const isFollowingRef = useRef(true)

  const { updateSessionMetadata } = useSessionActions()

  /**
   * Check if we're at the very bottom of the scroll container
   */
  const isAtBottom = () => {
    if (!messagesListRef.current) return true

    const { scrollTop, scrollHeight, clientHeight } = messagesListRef.current

    // No scrollbar? We're at "bottom"
    if (scrollHeight <= clientHeight) return true

    // Within 1px of bottom (accounts for sub-pixel rounding)
    return scrollHeight - scrollTop - clientHeight <= 1
  }

  // Effect: Listen for manual scrolling and save position (debounced)
  useEffect(() => {
    const container = messagesListRef.current
    if (!container || !activeSessionId) return

    let saveTimeout = null

    const handleScroll = () => {
      // Update following status immediately
      isFollowingRef.current = isAtBottom()

      // Debounce scroll position saving (avoid excessive state updates)
      if (saveTimeout) clearTimeout(saveTimeout)
      saveTimeout = setTimeout(() => {
        const scrollPosition = container.scrollTop
        updateSessionMetadata(activeSessionId, { scrollPosition })
      }, 200)
    }

    container.addEventListener('scroll', handleScroll, { passive: true })
    return () => {
      container.removeEventListener('scroll', handleScroll)
      if (saveTimeout) clearTimeout(saveTimeout)
    }
  }, [activeSessionId, updateSessionMetadata])

  // Effect: Handle session switching and restore scroll position
  useEffect(() => {
    const didSwitchSession =
      prevSessionIdRef.current !== null &&
      prevSessionIdRef.current !== activeSessionId

    prevSessionIdRef.current = activeSessionId

    if (didSwitchSession && activeSession && messages.length > 0) {
      setTimeout(() => {
        const savedScrollPosition = activeSession.scrollPosition

        if (savedScrollPosition !== undefined && savedScrollPosition !== null) {
          // Restore saved scroll position
          messagesListRef.current?.scrollTo({
            top: savedScrollPosition,
            behavior: 'instant',
          })
          // Restore following status based on saved position
          isFollowingRef.current = isAtBottom()
        } else if (activeSession.type === SESSION_TYPES.SHARED) {
          // Shared conversation, first load: start at top
          messagesListRef.current?.scrollTo({ top: 0, behavior: 'instant' })
          isFollowingRef.current = false
        } else {
          // Your conversations, first load: go to bottom
          messagesEndRef.current?.scrollIntoView({ behavior: 'instant' })
          isFollowingRef.current = true
        }
      }, 100)
    }
  }, [activeSessionId, activeSession, messages.length])

  // Effect: Handle new messages
  useEffect(() => {
    if (!settings.autoScroll || messages.length === 0) return
    if (activeSession?.type === SESSION_TYPES.SHARED) return

    const lastMessage = messages[messages.length - 1]

    // User sent a message? Always scroll and mark as following
    if (lastMessage?.type === MESSAGE_TYPES.USER) {
      isFollowingRef.current = true
      setTimeout(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
      }, 50)
      return
    }

    // Assistant/system message? Only scroll if following
    if (!isFollowingRef.current) return

    const isAssistantMessage =
      lastMessage?.type === MESSAGE_TYPES.ASSISTANT ||
      lastMessage?.type === MESSAGE_TYPES.FINAL_RESPONSE

    setTimeout(() => {
      if (isAssistantMessage && lastMessageRef.current) {
        // Scroll to show the assistant's message
        lastMessageRef.current.scrollIntoView({
          behavior: 'smooth',
          block: 'start',
        })
      } else {
        // Other messages: scroll to bottom
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
      }
    }, 50)
  }, [messages.length, settings.autoScroll, activeSession?.type])

  return {
    messagesEndRef,
    messagesListRef,
    lastMessageRef,
  }
}
