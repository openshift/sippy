import { Badge, Fab, Paper, Tooltip, Typography, Zoom } from '@mui/material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useEffect, useState } from 'react'
import sippyLogo from '../sippy.svg'

const useStyles = makeStyles((theme) => ({
  container: {
    position: 'fixed',
    bottom: theme.spacing(3),
    right: theme.spacing(3),
    zIndex: theme.zIndex.speedDial,
    display: 'flex',
    alignItems: 'flex-end',
    gap: theme.spacing(1),
    [theme.breakpoints.down('sm')]: {
      bottom: theme.spacing(2),
      right: theme.spacing(2),
    },
  },
  fab: {
    flexShrink: 0,
    backgroundColor: '#ffffff !important',
    '&:hover': {
      backgroundColor: '#f5f5f5 !important',
    },
  },
  sippyLogo: {
    width: 36,
    height: 36,
  },
  greetingBubble: {
    maxWidth: 250,
    padding: theme.spacing(1.5),
    backgroundColor: theme.palette.background.paper,
    boxShadow: theme.shadows[3],
    borderRadius: theme.spacing(1),
    position: 'relative',
    '&::after': {
      content: '""',
      position: 'absolute',
      right: -8,
      bottom: 16,
      width: 0,
      height: 0,
      borderLeft: `8px solid ${theme.palette.background.paper}`,
      borderTop: '8px solid transparent',
      borderBottom: '8px solid transparent',
    },
  },
  '@keyframes ripple': {
    '0%': {
      transform: 'scale(.8)',
      opacity: 1,
    },
    '100%': {
      transform: 'scale(2.4)',
      opacity: 0,
    },
  },
  badge: {
    '& .MuiBadge-badge': {
      backgroundColor: theme.palette.success.main,
      color: theme.palette.success.main,
      boxShadow: `0 0 0 2px ${theme.palette.background.paper}`,
      '&::after': {
        position: 'absolute',
        top: 0,
        left: 0,
        width: '100%',
        height: '100%',
        borderRadius: '50%',
        animation: '$ripple 1.2s infinite ease-in-out',
        border: '1px solid currentColor',
        content: '""',
      },
    },
  },
}))

export default function FloatingChatButton({
  onClick,
  unreadCount = 0,
  hasContext = false,
  disabled = false,
}) {
  const classes = useStyles()
  const [showGreeting, setShowGreeting] = useState(false)

  // Show greeting once per day
  useEffect(() => {
    const lastGreetingTime = localStorage.getItem('sippyChatLastGreeting')
    const now = Date.now()
    const oneDayMs = 24 * 60 * 60 * 1000 // 24 hours in milliseconds

    // Show if never shown, or if more than 24 hours since last greeting
    const shouldShowGreeting =
      !lastGreetingTime || now - parseInt(lastGreetingTime) > oneDayMs

    if (shouldShowGreeting) {
      // Wait 1 second before showing greeting so user notices it
      const showTimer = setTimeout(() => {
        setShowGreeting(true)
      }, 1000)

      // Hide after 8 seconds total (1s delay + 7s visible)
      const hideTimer = setTimeout(() => {
        setShowGreeting(false)
        localStorage.setItem('sippyChatLastGreeting', now.toString())
      }, 9000)

      return () => {
        clearTimeout(showTimer)
        clearTimeout(hideTimer)
      }
    }
  }, [])

  const getTooltipText = () => {
    if (disabled) return 'Chat unavailable'
    if (hasContext) return 'Open chat (with page context)'
    return 'Open chat'
  }

  return (
    <div className={classes.container}>
      <Zoom in={showGreeting} timeout={300}>
        <Paper className={classes.greetingBubble} elevation={3}>
          <Typography variant="body2">
            👋 Hi! I&apos;m Sippy. Click here to chat with me about CI jobs,
            test failures, and payloads!
          </Typography>
        </Paper>
      </Zoom>

      <Zoom in={true} timeout={300}>
        <Tooltip title={getTooltipText()} placement="left" arrow>
          <span>
            <Fab
              aria-label="chat"
              className={classes.fab}
              onClick={onClick}
              disabled={disabled}
            >
              <Badge
                badgeContent={unreadCount}
                color="error"
                overlap="circular"
                className={hasContext ? classes.badge : ''}
                variant={hasContext && unreadCount === 0 ? 'dot' : 'standard'}
              >
                <img
                  src={sippyLogo}
                  alt="Sippy"
                  className={classes.sippyLogo}
                />
              </Badge>
            </Fab>
          </span>
        </Tooltip>
      </Zoom>
    </div>
  )
}

FloatingChatButton.propTypes = {
  onClick: PropTypes.func.isRequired,
  unreadCount: PropTypes.number,
  hasContext: PropTypes.bool,
  disabled: PropTypes.bool,
}
