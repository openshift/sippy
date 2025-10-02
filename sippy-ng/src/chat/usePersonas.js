import { useCallback, useEffect, useState } from 'react'

/**
 * Custom hook to fetch and manage available personas from the API
 */
export function usePersonas() {
  const [personas, setPersonas] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [currentPersona, setCurrentPersona] = useState('default')

  // Get API base URL
  const getApiBaseUrl = useCallback(() => {
    const apiUrl =
      process.env.REACT_APP_CHAT_API_URL || window.location.origin + '/api/chat'
    // Remove trailing slash and /stream if present
    return apiUrl.replace(/\/$/, '').replace(/\/stream$/, '')
  }, [])

  // Fetch personas from API
  const fetchPersonas = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      const baseUrl = getApiBaseUrl()
      const response = await fetch(`${baseUrl}/personas`)

      if (!response.ok) {
        throw new Error(`Failed to fetch personas: ${response.statusText}`)
      }

      const data = await response.json()
      setPersonas(data.personas || [])
      setCurrentPersona(data.current_persona || 'default')
    } catch (err) {
      console.error('Error fetching personas:', err)
      setError(err.message)
      // Set default persona on error
      setPersonas([
        {
          name: 'default',
          description: 'Standard Sippy AI assistant',
          style_instructions: 'Professional and straightforward communication',
        },
      ])
    } finally {
      setLoading(false)
    }
  }, [getApiBaseUrl])

  // Fetch on mount
  useEffect(() => {
    fetchPersonas()
  }, [fetchPersonas])

  // Reload personas
  const reload = useCallback(() => {
    fetchPersonas()
  }, [fetchPersonas])

  return {
    personas,
    currentPersona,
    loading,
    error,
    reload,
  }
}
