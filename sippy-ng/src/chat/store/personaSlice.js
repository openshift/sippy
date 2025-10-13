/**
 * Persona slice - manages available chat personas
 */
export const createPersonaSlice = (set, get) => ({
  personas: [],
  personasLoading: false,
  personasError: null,

  loadPersonas: async () => {
    const apiUrl =
      process.env.REACT_APP_CHAT_API_URL || window.location.origin + '/api/chat'
    const baseUrl = apiUrl.replace(/\/$/, '').replace(/\/stream$/, '')

    try {
      set({ personasLoading: true, personasError: null })

      const response = await fetch(`${baseUrl}/personas`)

      if (!response.ok) {
        throw new Error(`Failed to fetch personas: ${response.statusText}`)
      }

      const data = await response.json()
      set({
        personas: data.personas || [],
        personasLoading: false,
      })
    } catch (err) {
      console.error('Error fetching personas:', err)
      set({
        personasError: err.message,
        personas: [],
        personasLoading: false,
      })
    }
  },
})
