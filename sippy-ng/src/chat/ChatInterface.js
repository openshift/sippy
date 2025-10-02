import {
  AddCircleOutline as AddCircleOutlineIcon,
  ExpandMore as ExpandMoreIcon,
  Masks as MasksIcon,
  Fullscreen as MaximizeIcon,
  FullscreenExit as MinimizeIcon,
  Settings as SettingsIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material'
import {
  Alert,
  Chip,
  Drawer,
  Fade,
  IconButton,
  Paper,
  Tooltip,
  Typography,
} from '@mui/material'
import { makeStyles } from '@mui/styles'
import { MESSAGE_TYPES } from './chatUtils'
import { useChatInterface } from './useChatInterface'
import ChatInput from './ChatInput'
import ChatMessage from './ChatMessage'
import ChatSettings from './ChatSettings'
import PropTypes from 'prop-types'
import React, { useEffect, useState } from 'react'
import SippyLogo from '../components/SippyLogo'
import ThinkingStep from './ThinkingStep'

const DRAWER_HEIGHT = 600
const DRAWER_HEIGHT_MAXIMIZED = '90vh'

const useStyles = makeStyles((theme) => ({
  // Full page styles
  fullPageRoot: {
    height: 'calc(100vh - 64px - 48px)',
    display: 'flex',
    flexDirection: 'column',
    backgroundColor: theme.palette.background.default,
    overflow: 'hidden',
    margin: -theme.spacing(3),
    marginTop: 0,
  },
  fullPageHeader: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    backgroundColor: theme.palette.background.paper,
  },

  // Drawer styles
  drawer: {
    flexShrink: 0,
  },
  drawerPaper: {
    height: DRAWER_HEIGHT,
    width: 600,
    maxWidth: '100vw',
    right: theme.spacing(3),
    left: 'auto',
    borderTopLeftRadius: theme.shape.borderRadius * 2,
    borderTopRightRadius: theme.shape.borderRadius * 2,
    borderTop: `2px solid ${theme.palette.divider}`,
    borderLeft: `1px solid ${theme.palette.divider}`,
    borderRight: `1px solid ${theme.palette.divider}`,
    boxShadow: theme.shadows[8],
    transition: theme.transitions.create(['height', 'width'], {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.enteringScreen,
    }),
  },
  drawerPaperMaximized: {
    height: DRAWER_HEIGHT_MAXIMIZED,
    width: '80vw',
  },
  drawerHeader: {
    display: 'flex',
    alignItems: 'center',
    padding: theme.spacing(2),
    justifyContent: 'space-between',
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    flexWrap: 'nowrap',
    overflow: 'hidden',
  },

  // Shared styles
  headerActions: {
    display: 'flex',
    gap: theme.spacing(1),
    alignItems: 'center',
    flexShrink: 0,
  },
  headerTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    minWidth: 0,
    flexShrink: 1,
    overflow: 'hidden',
  },
  messagesContainer: {
    flex: 1,
    overflowY: 'auto',
    padding: theme.spacing(2),
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(2),
  },
  emptyState: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    height: '100%',
    padding: theme.spacing(4),
    textAlign: 'center',
  },
  messageWrapper: {
    marginBottom: theme.spacing(2),
  },
  currentThinking: {
    margin: theme.spacing(1, 2),
    padding: theme.spacing(1),
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    borderRadius: theme.shape.borderRadius,
  },
  errorAlert: {
    margin: theme.spacing(1, 2),
    flexShrink: 0,
  },
  inputContainer: {
    flexShrink: 0,
  },
}))

/**
 * Unified chat interface component that can render as either:
 * - A full-page chat view (mode='fullPage')
 * - A drawer widget (mode='drawer')
 */
export default function ChatInterface({
  mode = 'fullPage',
  open = true,
  onClose,
  pageContext = null,
}) {
  const classes = useStyles()
  const [isMaximized, setIsMaximized] = useState(false)

  // Use shared chat interface logic
  const {
    settings,
    settingsOpen,
    setSettingsOpen,
    messages,
    filteredMessages,
    connectionState,
    currentThinking,
    error,
    isTyping,
    isConnected,
    personas,
    messagesEndRef,
    messagesListRef,
    lastMessageRef,
    handleSendMessage,
    handleClearMessages,
    handleReconnect,
    handleSettingsChange,
    getCurrentPersonaDisplay,
    getCurrentPersonaTooltip,
  } = useChatInterface()

  // Set page title for full page mode
  useEffect(() => {
    if (mode === 'fullPage') {
      document.title = 'Sippy > Chat Agent'
      return () => {
        document.title = 'Sippy'
      }
    }
  }, [mode])

  const getContextDisplay = () => {
    if (!pageContext || !pageContext.page) {
      return null
    }
    const pageName = pageContext.page
      .split('-')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
    return pageName
  }

  const renderEmptyState = () => (
    <div className={classes.emptyState}>
      <SippyLogo />
      <Typography variant={mode === 'fullPage' ? 'h5' : 'h6'} gutterBottom>
        Sippy Chat Assistant
      </Typography>
      <Typography variant="body2" color="textSecondary" paragraph>
        I can help you analyze jobs, investigate failures, check payloads, and
        more.
      </Typography>
    </div>
  )

  const renderMessages = () => {
    if (filteredMessages.length === 0 && !currentThinking) {
      return renderEmptyState()
    }

    return (
      <>
        {filteredMessages.map((msg, index) => {
          const isLastMessage = index === filteredMessages.length - 1
          if (msg.type === MESSAGE_TYPES.THINKING_STEP && msg.data) {
            return (
              <div
                key={index}
                className={classes.messageWrapper}
                ref={isLastMessage ? lastMessageRef : null}
              >
                <ThinkingStep data={msg.data} />
              </div>
            )
          }
          if (msg.type !== MESSAGE_TYPES.THINKING_STEP) {
            return (
              <div
                key={index}
                className={classes.messageWrapper}
                ref={isLastMessage ? lastMessageRef : null}
              >
                <ChatMessage message={msg} />
              </div>
            )
          }
          return null
        })}

        {currentThinking && settings.showThinking && (
          <Fade in timeout={300}>
            <div className={classes.currentThinking} ref={lastMessageRef}>
              <ThinkingStep data={currentThinking} isInProgress={true} />
            </div>
          </Fade>
        )}

        {error && (
          <Alert severity="error" className={classes.errorAlert}>
            {error}
          </Alert>
        )}

        <div ref={messagesEndRef} />
      </>
    )
  }

  const renderHeader = () => {
    const contextDisplay = getContextDisplay()

    return (
      <div
        className={
          mode === 'fullPage' ? classes.fullPageHeader : classes.drawerHeader
        }
      >
        <div className={classes.headerTitle}>
          <SmartToyIcon sx={{ flexShrink: 0 }} />
          <Typography
            variant="h6"
            sx={{
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              minWidth: 0,
            }}
          >
            {mode === 'fullPage' ? 'Chat Agent' : 'Chat Assistant'}
          </Typography>
        </div>

        <div className={classes.headerActions}>
          <Tooltip title="New chat">
            <IconButton size="small" onClick={handleClearMessages}>
              <AddCircleOutlineIcon />
            </IconButton>
          </Tooltip>

          <Tooltip title="Settings">
            <IconButton size="small" onClick={() => setSettingsOpen(true)}>
              <SettingsIcon />
            </IconButton>
          </Tooltip>

          {mode === 'drawer' && (
            <>
              <Tooltip title={isMaximized ? 'Minimize' : 'Maximize'}>
                <IconButton
                  size="small"
                  onClick={() => setIsMaximized(!isMaximized)}
                >
                  {isMaximized ? <MinimizeIcon /> : <MaximizeIcon />}
                </IconButton>
              </Tooltip>

              <Tooltip title="Minimize">
                <IconButton size="small" onClick={onClose}>
                  <ExpandMoreIcon />
                </IconButton>
              </Tooltip>
            </>
          )}
        </div>
      </div>
    )
  }

  const renderContent = () => (
    <>
      {renderHeader()}

      <div ref={messagesListRef} className={classes.messagesContainer}>
        {renderMessages()}
      </div>

      <div className={classes.inputContainer}>
        <ChatInput
          onSendMessage={handleSendMessage}
          disabled={!isConnected}
          isConnected={isConnected}
          isTyping={isTyping}
          onRetry={handleReconnect}
          suggestedQuestions={pageContext?.suggestedQuestions || null}
          contextChip={
            pageContext && getContextDisplay() ? (
              <Tooltip title={`Context: ${getContextDisplay()}`}>
                <Chip
                  label={getContextDisplay()}
                  size="small"
                  color="primary"
                  variant="outlined"
                />
              </Tooltip>
            ) : null
          }
          personaChip={
            personas.length > 0 && settings.persona !== 'default' ? (
              <Tooltip title={getCurrentPersonaTooltip()}>
                <Chip
                  icon={<MasksIcon />}
                  label={getCurrentPersonaDisplay()}
                  size="small"
                  color="secondary"
                  variant="outlined"
                />
              </Tooltip>
            ) : null
          }
        />
      </div>

      <ChatSettings
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings}
        onSettingsChange={handleSettingsChange}
        onClearMessages={handleClearMessages}
        onReconnect={handleReconnect}
        connectionState={connectionState}
        messageCount={messages.length}
        isConnected={isConnected}
      />
    </>
  )

  if (mode === 'drawer') {
    return (
      <Drawer
        className={classes.drawer}
        variant="persistent"
        anchor="bottom"
        open={open}
        classes={{
          paper: `${classes.drawerPaper} ${
            isMaximized ? classes.drawerPaperMaximized : ''
          }`,
        }}
      >
        {renderContent()}
      </Drawer>
    )
  }

  // Full page mode
  return <Paper className={classes.fullPageRoot}>{renderContent()}</Paper>
}

ChatInterface.propTypes = {
  mode: PropTypes.oneOf(['fullPage', 'drawer']),
  open: PropTypes.bool,
  onClose: PropTypes.func,
  pageContext: PropTypes.shape({
    page: PropTypes.string,
    url: PropTypes.string,
    data: PropTypes.object,
    instructions: PropTypes.string,
    suggestedQuestions: PropTypes.arrayOf(PropTypes.string),
  }),
}
