/**
 * Persona slice - manages available chat personas
 */
export const createPersonaSlice = (set, get) => ({
  personas: [],
  personasLoading: false,
  personasError: null,

  loadPersonas: () => {
    const apiUrl =
      process.env.REACT_APP_CHAT_API_URL || window.location.origin + '/api/chat'
    const baseUrl = apiUrl.replace(/\/$/, '').replace(/\/stream$/, '')

    set({ personasLoading: true, personasError: null })

    fetch(`${baseUrl}/personas`)
      .then((response) => {
        if (!response.ok) {
          throw new Error(`Failed to fetch personas: ${response.statusText}`)
        }
        return response.json()
      })
      .then((data) => {
        set({
          personas: data.personas || [],
          personasLoading: false,
        })
      })
      .catch((err) => {
        console.error('Error fetching personas:', err)
        set({
          personasError: err.message,
          personas: [],
          personasLoading: false,
        })
      })
  },
})
