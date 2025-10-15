/**
 * Settings slice - manages user preferences
 */
export const createSettingsSlice = (set, get) => ({
  // State
  settings: {
    showThinking: true,
    autoScroll: true,
    persona: 'default',
    tourCompleted: false,
    clientId: null,
  },
  settingsOpen: false,

  // Actions
  updateSettings: (newSettings) => {
    set((state) => ({
      settings: {
        ...state.settings,
        ...newSettings,
      },
    }))
  },

  // Initialize client ID if not already set (called on app mount)
  ensureClientId: () => {
    const state = get()
    if (!state.settings.clientId) {
      set((state) => ({
        settings: {
          ...state.settings,
          clientId: crypto.randomUUID(),
        },
      }))
    }
  },

  setSettingsOpen: (open) => {
    set({ settingsOpen: open })
  },

  // Tour actions
  setTourCompleted: (completed) => {
    set((state) => ({
      settings: {
        ...state.settings,
        tourCompleted: completed,
      },
    }))
  },

  resetTour: () => {
    set((state) => ({
      settings: {
        ...state.settings,
        tourCompleted: false,
      },
    }))
  },
})
