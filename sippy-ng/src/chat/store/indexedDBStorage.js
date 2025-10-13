import { del, get, set } from 'idb-keyval'

/**
 * IndexedDB storage adapter for Zustand persist middleware
 * This provides a drop-in replacement for localStorage with much larger quota
 * and better performance for large datasets.
 *
 * Implementation follows the official Zustand pattern:
 * https://docs.pmnd.rs/zustand/integrations/persisting-store-data#how-can-i-use-a-custom-storage-engine
 */
const indexedDBStorage = {
  getItem: async (name) => {
    try {
      const value = await get(name)
      return value || null
    } catch (error) {
      console.error('Error reading from IndexedDB:', error)
      return null
    }
  },
  setItem: async (name, value) => {
    try {
      await set(name, value)
    } catch (error) {
      console.error('Error writing to IndexedDB:', error)
      // If we hit quota, we could implement cleanup logic here
      throw error
    }
  },
  removeItem: async (name) => {
    try {
      await del(name)
    } catch (error) {
      console.error('Error removing from IndexedDB:', error)
      throw error
    }
  },
}

export default indexedDBStorage
