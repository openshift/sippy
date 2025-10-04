import { createContext, useCallback, useContext, useState } from 'react'
import { DEFAULT_CHAT_SETTINGS } from './chatUtils'
import { useChatWebSocket } from './useChatWebSocket'
import { useCookies } from 'react-cookie'
import PropTypes from 'prop-types'
import React from 'react'

const GlobalChatContext = createContext(null)

export function GlobalChatProvider({ children }) {
  const [isOpen, setIsOpen] = useState(false)
  const [pageContext, setPageContext] = useState(null)
  const [unreadCount, setUnreadCount] = useState(0)

  // Load settings from cookie
  const [cookies] = useCookies(['sippyChatSettings'])
  const settings = cookies.sippyChatSettings || DEFAULT_CHAT_SETTINGS

  // Shared WebSocket connection and messages
  console.log('GlobalChatProvider rendering with pageContext:', pageContext)
  const chatWebSocket = useChatWebSocket(settings, pageContext)

  const openChat = useCallback((context) => {
    // Only update pageContext if explicitly provided and is a valid object
    if (context && typeof context === 'object' && !context.nativeEvent) {
      setPageContext(context)
    }
    setIsOpen(true)
    setUnreadCount(0) // Clear unread count when opening
  }, [])

  const closeChat = useCallback(() => {
    setIsOpen(false)
  }, [])

  const toggleChat = useCallback(
    (context = undefined) => {
      if (isOpen) {
        closeChat()
      } else {
        openChat(context)
      }
    },
    [isOpen, closeChat, openChat]
  )

  const updatePageContext = useCallback((context) => {
    console.log('updatePageContext called with:', context)
    setPageContext(context)
  }, [])

  const incrementUnreadCount = useCallback(() => {
    setUnreadCount((prev) => prev + 1)
  }, [])

  const askQuestion = useCallback(
    (question, context) => {
      // Update context only if explicitly provided and is a valid object
      if (context && typeof context === 'object' && !context.nativeEvent) {
        setPageContext(context)
      }
      // Open chat
      setIsOpen(true)
      setUnreadCount(0)
      // Send the message after a brief delay to ensure chat is rendered
      setTimeout(() => {
        chatWebSocket.sendMessage(question)
      }, 100)
    },
    [chatWebSocket]
  )

  return (
    <GlobalChatContext.Provider
      value={{
        isOpen,
        pageContext,
        unreadCount,
        openChat,
        closeChat,
        toggleChat,
        updatePageContext,
        incrementUnreadCount,
        askQuestion,
        // Expose chat WebSocket functionality
        ...chatWebSocket,
      }}
    >
      {children}
    </GlobalChatContext.Provider>
  )
}

GlobalChatProvider.propTypes = {
  children: PropTypes.node.isRequired,
}

export function useGlobalChat() {
  const context = useContext(GlobalChatContext)
  if (!context) {
    throw new Error('useGlobalChat must be used within a GlobalChatProvider')
  }
  return context
}
