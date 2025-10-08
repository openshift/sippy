import { DEFAULT_CHAT_SETTINGS, MESSAGE_TYPES } from './chatUtils'
import { useCookies } from 'react-cookie'
import { useEffect, useRef, useState } from 'react'
import { useGlobalChat } from './useGlobalChat'
import { usePersonas } from './usePersonas'

/**
 * Shared hook for chat interface logic used by both ChatAgent and GlobalChatWidget
 */
export function useChatInterface() {
  const [cookies, setCookie] = useCookies(['sippyChatSettings'])

  // Load settings from cookie or use defaults
  const [settings, setSettings] = useState(() => {
    if (cookies.sippyChatSettings) {
      return { ...DEFAULT_CHAT_SETTINGS, ...cookies.sippyChatSettings }
    }
    return DEFAULT_CHAT_SETTINGS
  })

  const [settingsOpen, setSettingsOpen] = useState(false)
  const messagesEndRef = useRef(null)
  const messagesListRef = useRef(null)
  const lastMessageRef = useRef(null)

  // Use shared chat state from global context
  const {
    messages,
    connectionState,
    currentThinking,
    error,
    isTyping,
    sendMessage,
    clearMessages,
    connect,
    disconnect,
    isConnected,
  } = useGlobalChat()

  const { personas } = usePersonas()

  // Helper functions
  const getCurrentPersonaDisplay = () => {
    const personaName = settings.persona || 'default'
    return (
      personaName.charAt(0).toUpperCase() +
      personaName.slice(1).replace(/_/g, ' ')
    )
  }

  const getCurrentPersonaTooltip = () => {
    const persona = personas.find((p) => p.name === settings.persona)
    return persona ? persona.description : 'Default AI assistant'
  }

  // Auto-scroll to show the latest message when new messages arrive
  useEffect(() => {
    if (settings.autoScroll && lastMessageRef.current) {
      // Scroll to the top of the last message, not the bottom
      lastMessageRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
      })
    }
  }, [messages, currentThinking, settings.autoScroll])

  // Event handlers
  const handleSendMessage = (content) => {
    return sendMessage(content)
  }

  const handleClearMessages = () => {
    clearMessages()
    setSettingsOpen(false)
  }

  const handleReconnect = () => {
    disconnect()
    setTimeout(() => connect(), 1000)
  }

  const handleSettingsChange = (newSettings) => {
    setSettings(newSettings)
    // Save settings to cookie
    setCookie('sippyChatSettings', newSettings, {
      path: '/',
      sameSite: 'Strict',
      expires: new Date('3000-12-31'),
    })
  }

  // Filter messages based on settings
  const filteredMessages = settings.showThinking
    ? messages
    : messages.filter((msg) => msg.type !== MESSAGE_TYPES.THINKING_STEP)

  return {
    // State
    settings,
    settingsOpen,
    setSettingsOpen,
    messages, // Keep for components that need it
    filteredMessages,
    connectionState, // Keep for components that need it
    currentThinking,
    error,
    isTyping,
    isConnected,
    personas,

    // Refs
    messagesEndRef,
    messagesListRef,
    lastMessageRef,

    // Handlers
    handleSendMessage,
    handleClearMessages,
    handleReconnect,
    handleSettingsChange,

    // Helpers
    getCurrentPersonaDisplay,
    getCurrentPersonaTooltip,
  }
}
