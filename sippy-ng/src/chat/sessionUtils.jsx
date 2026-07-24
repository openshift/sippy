import { SESSION_TYPES } from './store/sessionSlice'

/**
 * Get a display title for a session (first message preview)
 * @param {Object} session - Session object with messages array
 * @returns {string} Display title for the session
 */
export function getSessionTitle(session) {
  const firstUserMessage = session?.messages?.find((msg) => msg.type === 'user')
  if (firstUserMessage?.content) {
    const content = firstUserMessage.content.trim()
    return content.length > 40 ? content.substring(0, 40) + '...' : content
  }
  return 'Untitled Conversation'
}

/**
 * Get icon name for session type (Material UI icon)
 * @param {string} sessionType - Session type from SESSION_TYPES
 * @returns {string} Material UI icon name
 */
export function getSessionIconName(sessionType) {
  switch (sessionType) {
    case SESSION_TYPES.OWN:
      return 'Chat'
    case SESSION_TYPES.SHARED:
      return 'Person'
    case SESSION_TYPES.SHARED_BY_ME:
      return 'Share'
    case SESSION_TYPES.FORKED:
      return 'CallSplit'
    default:
      return 'Chat'
  }
}
