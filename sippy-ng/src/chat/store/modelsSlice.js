/**
 * Models slice - manages available AI models
 */
export const createModelsSlice = (set, get) => ({
  // State
  models: [],
  defaultModel: null,
  modelsLoading: false,
  modelsError: null,

  // Actions
  loadModels: () => {
    const apiUrl =
      process.env.REACT_APP_CHAT_API_URL || window.location.origin + '/api/chat'
    const baseUrl = apiUrl.replace(/\/$/, '').replace(/\/stream$/, '')

    set({ modelsLoading: true, modelsError: null })

    fetch(`${baseUrl}/models`)
      .then((response) => {
        if (!response.ok) {
          throw new Error(`Failed to load models: ${response.statusText}`)
        }
        return response.json()
      })
      .then((data) => {
        set({
          models: data.models || [],
          defaultModel: data.default_model,
          modelsLoading: false,
        })

        // If user hasn't selected a model yet, set to default
        const currentSettings = get().settings
        if (!currentSettings.modelId && data.default_model) {
          get().updateSettings({ modelId: data.default_model })
        }
      })
      .catch((error) => {
        console.error('Error loading models:', error)
        set({
          modelsError: error.message,
          modelsLoading: false,
          // Fallback to a default model if loading fails
          models: [],
          defaultModel: null,
        })
      })
  },
})
