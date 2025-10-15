import { MESSAGE_TYPES } from './chatUtils'
import { useCallback } from 'react'
import { useSessionActions, useSettings } from './store/useChatStore'

/**
 * Custom hook for handling session rating submission
 * Calculates metrics and submits anonymous ratings to the API
 */
export function useSessionRating() {
  const { updateSessionMetadata } = useSessionActions()
  const { settings } = useSettings()

  /**
   * Calculate metrics from messages for rating submission
   */
  const calculateMetrics = useCallback((messages) => {
    const userMessages = messages.filter(
      (msg) => msg.type === MESSAGE_TYPES.USER
    )
    const assistantMessages = messages.filter(
      (msg) => msg.type === MESSAGE_TYPES.ASSISTANT
    )
    const thinkingSteps = messages.filter(
      (msg) => msg.type === MESSAGE_TYPES.THINKING_STEP
    )

    const llmThoughts = thinkingSteps.filter(
      (msg) => msg.data?.action === 'thinking'
    ).length

    const toolCalls = thinkingSteps.filter(
      (msg) => msg.data?.action && msg.data.action !== 'thinking'
    ).length

    const totalMessages = userMessages.length + assistantMessages.length
    const interactionSize = new Blob([JSON.stringify(messages)]).size

    return {
      totalMessages,
      userMessages: userMessages.length,
      assistantMessages: assistantMessages.length,
      toolCalls,
      llmThoughts,
      totalSizeBytes: interactionSize,
    }
  }, [])

  /**
   * Submit a rating for a session
   * @param {string} sessionId - The session ID
   * @param {string} sessionType - The type of session (SESSION_TYPES)
   * @param {Array} messages - The messages in the session
   * @param {number} rating - The rating value (1-5)
   */
  const submitRating = useCallback(
    async (sessionId, sessionType, messages, rating) => {
      if (!sessionId) {
        console.error('No session ID provided for rating')
        return { success: false, error: 'No session ID' }
      }

      // Calculate metrics
      const metrics = calculateMetrics(messages)

      const payload = {
        rating,
        clientId: settings.clientId,
        metadata: {
          ...metrics,
          sessionType,
          timestamp: new Date().toISOString(),
        },
      }

      try {
        // Submit to API
        const response = await fetch(
          process.env.REACT_APP_API_URL + '/api/chat/ratings',
          {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify(payload),
          }
        )

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`)
        }

        const data = await response.json()

        // Update session metadata after successful submission
        // Delay to allow thank you message and fade out animation (2s + 0.5s)
        setTimeout(() => {
          updateSessionMetadata(sessionId, {
            rated: true,
            ratedAt: new Date().toISOString(),
            ratingValue: rating,
          })
        }, 2500)

        return { success: true, data }
      } catch (error) {
        console.error('Failed to submit rating:', error)
        return { success: false, error: error.message }
      }
    },
    [calculateMetrics, updateSessionMetadata, settings.clientId]
  )

  return {
    submitRating,
    calculateMetrics,
  }
}
