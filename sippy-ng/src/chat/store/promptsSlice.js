/**
 * Zustand slice for managing slash command prompts
 */
export const createPromptsSlice = (set, get) => ({
  // State
  prompts: [],
  promptsLoading: false,
  promptsError: null,

  // Fetch prompts from the server
  fetchPrompts: async () => {
    set({ promptsLoading: true, promptsError: null })

    try {
      const response = await fetch(
        process.env.REACT_APP_CHAT_API_URL + '/prompts' || '/api/chat/prompts'
      )

      if (!response.ok) {
        throw new Error('Failed to fetch prompts')
      }

      const data = await response.json()
      set({
        prompts: data.prompts || [],
        promptsLoading: false,
      })
    } catch (error) {
      set({
        promptsLoading: false,
        promptsError: error.message,
      })
    }
  },

  // Render a prompt with arguments
  renderPrompt: async (promptName, args) => {
    try {
      const response = await fetch(
        process.env.REACT_APP_CHAT_API_URL + '/prompts/render' ||
          '/api/chat/prompts/render',
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            prompt_name: promptName,
            arguments: args,
          }),
        }
      )

      if (!response.ok) {
        throw new Error('Failed to render prompt')
      }

      const data = await response.json()
      return data.rendered
    } catch (error) {
      console.error('Error rendering prompt:', error)
      throw error
    }
  },
})
