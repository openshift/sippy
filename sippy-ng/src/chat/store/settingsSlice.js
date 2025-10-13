/**
 * Settings slice - manages user preferences
 */
export const createSettingsSlice = (set, get) => ({
  // State
  settings: {
    showThinking: true,
    autoScroll: true,
    persona: 'default',
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
})
