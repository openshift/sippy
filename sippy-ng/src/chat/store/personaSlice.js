/**
 * Persona slice - manages available chat personas
 */
export const createPersonaSlice = (set, get) => ({
  personas: [],
  personasLoading: false,
  personasError: null,

  loadPersonas: () => {
    set({ personasLoading: true, personasError: null })

    fetch(process.env.REACT_APP_API_URL + '/api/chat/personas')
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
