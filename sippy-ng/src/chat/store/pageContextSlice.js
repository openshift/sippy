/**
 * Page context slice - manages the current page context for chat
 * This tracks what page the user is on and what data is available
 * for context-aware chat interactions
 */
export const createPageContextSlice = (set) => ({
  // State
  pageContext: null,

  // Actions
  setPageContextForChat: (context) => {
    console.log('Setting page context for chat:', context)
    set({ pageContext: context })
  },

  unsetPageContextForChat: () => {
    console.log('Clearing page context for chat')
    set({ pageContext: null })
  },
})
