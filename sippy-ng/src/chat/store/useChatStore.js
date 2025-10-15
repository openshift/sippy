import { create } from 'zustand'
import { createDrawerSlice } from './drawerSlice'
import { createJSONStorage, persist } from 'zustand/middleware'
import { createPageContextSlice } from './pageContextSlice'
import { createPersonaSlice } from './personaSlice'
import { createSessionSlice } from './sessionSlice'
import { createSettingsSlice } from './settingsSlice'
import { createShareSlice } from './shareSlice'
import { createWebSocketSlice } from './webSocketSlice'
import { useShallow } from 'zustand/react/shallow'
import indexedDBStorage from './indexedDBStorage'

/**
 * Zustand store for the chat interface, state is grouped in slices. Persistent
 * state is stored in IndexedDB; slices persisted are selected by partialization.
 *
 * Always useShallow() when creating more slices, to avoid re-rendering the entire
 * store when one of the values changes.
 */
export const useChatStore = create(
  persist(
    (set, get) => ({
      ...createSessionSlice(set, get),
      ...createShareSlice(set, get),
      ...createWebSocketSlice(set, get),
      ...createSettingsSlice(set, get),
      ...createPersonaSlice(set, get),
      ...createPageContextSlice(set, get),
      ...createDrawerSlice(set, get),
    }),
    {
      name: 'sippy-chat-storage',
      storage: createJSONStorage(() => indexedDBStorage),
      partialize: (state) => ({
        sessions: state.sessions,
        activeSessionId: state.activeSessionId,
        settings: state.settings,
      }),
    }
  )
)

/**
 * Grouped selectors for Zustand state
 * These provide cleaner access to related state and actions, make sure to always useShallow()
 * when creating more of these, to avoid re-rendering the entire store when one of the values changes
 */

export const useSessionState = () =>
  useChatStore(
    useShallow((state) => ({
      sessions: state.sessions,
      activeSessionId: state.activeSessionId,
      activeSession: state.getActiveSession(),
    }))
  )

export const useSessionActions = () =>
  useChatStore(
    useShallow((state) => ({
      initializeSessions: state.initializeSessions,
      createSession: state.createSession,
      switchSession: state.switchSession,
      startNewSession: state.startNewSession,
      deleteSession: state.deleteSession,
      updateSessionMetadata: state.updateSessionMetadata,
      forkActiveSession: state.forkActiveSession,
      clearAllSessions: state.clearAllSessions,
      clearOldSessions: state.clearOldSessions,
    }))
  )

export const useShareState = () =>
  useChatStore(
    useShallow((state) => ({
      shareLoading: state.shareLoading,
      shareDialogOpen: state.shareDialogOpen,
      sharedUrl: state.sharedUrl,
      shareSnackbar: state.shareSnackbar,
      loadingShared: state.loadingShared,
    }))
  )

export const useShareActions = () =>
  useChatStore(
    useShallow((state) => ({
      shareConversation: state.shareConversation,
      clearSharedUrl: state.clearSharedUrl,
      loadSharedConversationFromAPI: state.loadSharedConversationFromAPI,
      setShareDialogOpen: state.setShareDialogOpen,
      setShareSnackbar: state.setShareSnackbar,
      closeShareSnackbar: state.closeShareSnackbar,
      copyToClipboard: state.copyToClipboard,
    }))
  )

export const useConnectionState = () =>
  useChatStore(
    useShallow((state) => ({
      connectionState: state.connectionState,
      isTyping: state.isTyping,
      error: state.error,
      currentThinking: state.currentThinking,
    }))
  )

export const useSettings = () =>
  useChatStore(
    useShallow((state) => ({
      settings: state.settings,
      settingsOpen: state.settingsOpen,
      updateSettings: state.updateSettings,
      setSettingsOpen: state.setSettingsOpen,
      setTourCompleted: state.setTourCompleted,
      resetTour: state.resetTour,
    }))
  )

export const usePersonas = () =>
  useChatStore(
    useShallow((state) => ({
      personas: state.personas,
      personasLoading: state.personasLoading,
      personasError: state.personasError,
      loadPersonas: state.loadPersonas,
    }))
  )

export const usePageContextForChat = () =>
  useChatStore(
    useShallow((state) => ({
      pageContext: state.pageContext,
      setPageContextForChat: state.setPageContextForChat,
      unsetPageContextForChat: state.unsetPageContextForChat,
    }))
  )

export const useDrawer = () =>
  useChatStore(
    useShallow((state) => ({
      isDrawerOpen: state.isDrawerOpen,
      openDrawer: state.openDrawer,
      closeDrawer: state.closeDrawer,
      toggleDrawer: state.toggleDrawer,
    }))
  )

export const useWebSocketActions = () =>
  useChatStore(
    useShallow((state) => ({
      connectWebSocket: state.connectWebSocket,
      disconnectWebSocket: state.disconnectWebSocket,
      sendMessage: state.sendMessage,
      stopGeneration: state.stopGeneration,
    }))
  )
