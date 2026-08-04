import { get } from 'idb-keyval'

const STORAGE_KEY = 'sippy-chat-storage'

/**
 * Get storage statistics for chat data
 * @param {Array} sessions - Sessions array from the store
 * @param {string} activeSessionId - Active session ID
 * @param {Object} settings - Settings object
 * @returns {Promise<{conversationCount: number, sizeBytes: number}>}
 */
export async function getChatStorageStats(
  sessions = [],
  activeSessionId = null,
  settings = {}
) {
  try {
    const conversationCount = sessions.length

    // Get the full data from IndexedDB to calculate actual storage size
    const data = await get(STORAGE_KEY)
    let sizeBytes = 0

    if (data) {
      // Calculate size in bytes by serializing the actual stored data
      const serialized = JSON.stringify(data)
      sizeBytes = new Blob([serialized]).size
    } else {
      // Fallback: estimate size from current state if not in storage yet
      const estimatedData = {
        state: {
          sessions,
          activeSessionId,
          settings,
        },
      }
      const serialized = JSON.stringify(estimatedData)
      sizeBytes = new Blob([serialized]).size
    }

    return { conversationCount, sizeBytes }
  } catch (error) {
    console.error('Error getting chat storage stats:', error)
    return { conversationCount: sessions.length, sizeBytes: 0 }
  }
}

/**
 * Format bytes to human-readable format
 * @param {number} bytes
 * @param {number} decimals
 * @returns {string}
 */
export function formatBytes(bytes, decimals = 1) {
  if (bytes === 0) return '0 Bytes'

  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['Bytes', 'KB', 'MB', 'GB']

  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}
