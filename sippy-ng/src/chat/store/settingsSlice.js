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
