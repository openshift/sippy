import { Badge, Paper, Typography } from '@mui/material'
import { ExpandLess as ExpandLessIcon } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import ChatInterface from './ChatInterface'
import PropTypes from 'prop-types'
import React from 'react'
import sippyLogo from '../sippy.svg'

const useStyles = makeStyles((theme) => ({
  collapsedTab: {
    position: 'fixed',
    right: theme.spacing(3),
    bottom: 0,
    zIndex: theme.zIndex.drawer - 1,
    backgroundColor: theme.palette.background.paper,
    borderTopLeftRadius: theme.shape.borderRadius * 2,
    borderTopRightRadius: theme.shape.borderRadius * 2,
    borderTop: `2px solid ${theme.palette.divider}`,
    borderLeft: `1px solid ${theme.palette.divider}`,
    borderRight: `1px solid ${theme.palette.divider}`,
    boxShadow: theme.shadows[8],
    cursor: 'pointer',
    transition: 'all 0.3s ease',
    '&:hover': {
      boxShadow: theme.shadows[10],
      bottom: 0,
    },
  },
  collapsedContent: {
    display: 'flex',
    flexDirection: 'row',
    alignItems: 'center',
    padding: theme.spacing(1, 2),
    gap: theme.spacing(1.5),
  },
  horizontalText: {
    fontSize: '0.875rem',
    fontWeight: 500,
    color: theme.palette.text.primary,
    whiteSpace: 'nowrap',
  },
  sippyLogo: {
    width: 32,
    height: 32,
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
}))

export default function CollapsibleChatDrawer({
  open,
  onOpen,
  onClose,
  pageContext,
  hasContext,
  unreadCount,
}) {
  const classes = useStyles()

  const handleOpen = (e) => {
    e.preventDefault()
    e.stopPropagation()
    onOpen()
  }

  return (
    <>
      {/* Collapsed tab - shown when drawer is closed */}
      {!open && (
        <Paper
          className={classes.collapsedTab}
          onClick={handleOpen}
          elevation={3}
        >
          <div className={classes.collapsedContent}>
            <Badge
              badgeContent={unreadCount}
              color="error"
              overlap="circular"
              className={hasContext ? classes.badge : ''}
              variant={hasContext && unreadCount === 0 ? 'dot' : 'standard'}
            >
              <img src={sippyLogo} alt="Sippy" className={classes.sippyLogo} />
            </Badge>
            <Typography className={classes.horizontalText}>
              Chat with Sippy
            </Typography>
            <ExpandLessIcon fontSize="small" />
          </div>
        </Paper>
      )}

      {/* Full drawer - shown when open */}
      <ChatInterface
        mode="drawer"
        open={open}
        onClose={onClose}
        pageContext={pageContext}
      />
    </>
  )
}

CollapsibleChatDrawer.propTypes = {
  open: PropTypes.bool.isRequired,
  onOpen: PropTypes.func.isRequired,
  onClose: PropTypes.func.isRequired,
  pageContext: PropTypes.object,
  hasContext: PropTypes.bool,
  unreadCount: PropTypes.number,
}
