import {
  CircularProgress,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  ExpandMore as ExpandMoreIcon,
  Help as HelpIcon,
  Fullscreen as MaximizeIcon,
  FullscreenExit as RestoreIcon,
  Settings as SettingsIcon,
  Share as ShareIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import {
  useConnectionState,
  usePageContextForChat,
  useSessionState,
  useSettings,
  useShareActions,
  useShareState,
} from './store/useChatStore'
import PropTypes from 'prop-types'
import React from 'react'
import SessionManager from './SessionDropdown'

const useStyles = makeStyles((theme) => ({
  // Full page styles
  fullPageHeader: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    backgroundColor: theme.palette.background.paper,
  },

  // Drawer styles
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

  headerTitle: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: theme.spacing(1),
    minWidth: 0,
    flexShrink: 1,
    overflow: 'hidden',
  },
  headerTextContainer: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(0.25),
    minWidth: 0,
  },
  aiNotice: {
    fontSize: '0.65rem',
    color: theme.palette.text.secondary,
    fontStyle: 'italic',
    lineHeight: 1.2,
  },
  headerActions: {
    display: 'flex',
    gap: theme.spacing(1),
    alignItems: 'center',
    flexShrink: 0,
  },
}))

export default function ChatHeader({
  mode,
  onNewSession,
  onMaximize,
  onClose,
  isMaximized,
}) {
  const classes = useStyles()

  // Get state from custom hooks
  const { activeSession } = useSessionState()
  const { currentThinking, isTyping } = useConnectionState()
  const { shareLoading } = useShareState()
  const { shareConversation } = useShareActions()
  const { setSettingsOpen } = useSettings()
  const { pageContext } = usePageContextForChat()

  const messages = activeSession?.messages || []
  const hasMessages = messages.length > 0

  const handleHelp = () => {
    window.open(
      'https://source.redhat.com/departments/products_and_global_engineering/openshift_development/openshift_wiki/sippy_chat_user_guide',
      '_blank',
      'noopener,noreferrer'
    )
  }

  const handleShare = () => {
    shareConversation(pageContext, mode)
  }

  return (
    <div
      className={
        mode === 'fullPage' ? classes.fullPageHeader : classes.drawerHeader
      }
    >
      <div className={classes.headerTitle}>
        <SmartToyIcon sx={{ flexShrink: 0, marginTop: '4px' }} />
        <div className={classes.headerTextContainer}>
          <Typography
            variant="h6"
            sx={{
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              minWidth: 0,
            }}
          >
            Chat Assistant
          </Typography>
          <Typography
            className={classes.aiNotice}
            sx={{
              fontSize: '0.65rem !important',
            }}
          >
            Always review AI generated content prior to use.
          </Typography>
        </div>
      </div>

      <div className={classes.headerActions}>
        <SessionManager onNewSession={onNewSession} mode={mode} />

        <Tooltip title="Share conversation">
          <span>
            <IconButton
              size="small"
              onClick={handleShare}
              disabled={
                shareLoading || !hasMessages || isTyping || currentThinking
              }
              data-tour="share-button"
            >
              {shareLoading ? <CircularProgress size={20} /> : <ShareIcon />}
            </IconButton>
          </span>
        </Tooltip>

        <Tooltip title="Help">
          <IconButton size="small" onClick={handleHelp} data-tour="help-button">
            <HelpIcon />
          </IconButton>
        </Tooltip>

        <Tooltip title="Settings">
          <IconButton
            size="small"
            onClick={() => setSettingsOpen(true)}
            data-tour="settings-button"
          >
            <SettingsIcon />
          </IconButton>
        </Tooltip>

        {mode === 'drawer' && (
          <>
            <Tooltip title={isMaximized ? 'Restore' : 'Maximize'}>
              <IconButton size="small" onClick={onMaximize}>
                {isMaximized ? <RestoreIcon /> : <MaximizeIcon />}
              </IconButton>
            </Tooltip>

            <Tooltip title="Close">
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

ChatHeader.propTypes = {
  mode: PropTypes.oneOf(['fullPage', 'drawer']).isRequired,
  onNewSession: PropTypes.func.isRequired,
  onMaximize: PropTypes.func,
  onClose: PropTypes.func,
  isMaximized: PropTypes.bool,
}
