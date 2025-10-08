import { Box, Chip, IconButton, Paper, Typography } from '@mui/material'
import {
  Close as CloseIcon,
  Delete as DeleteIcon,
  Send as SendIcon,
} from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles((theme) => ({
  queueContainer: {
    padding: theme.spacing(1, 2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
  },
  queueHeader: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: theme.spacing(1),
  },
  queueTitle: {
    fontSize: '0.875rem',
    fontWeight: 500,
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  queuedMessage: {
    padding: theme.spacing(0.75, 1.5),
    marginBottom: theme.spacing(0.75),
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    fontSize: '0.875rem',
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    overflow: 'hidden',
  },
  messageText: {
    flex: 1,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  messageActions: {
    display: 'flex',
    gap: theme.spacing(0.5),
  },
}))

export default function QueuedMessages({
  queue = [],
  onSendNow,
  onDeleteMessage,
  onClearQueue,
}) {
  const classes = useStyles()

  if (queue.length === 0) {
    return null
  }

  return (
    <Box className={classes.queueContainer}>
      <div className={classes.queueHeader}>
        <Typography className={classes.queueTitle}>
          <Chip
            label={queue.length}
            size="small"
            color="warning"
            sx={{ height: 20 }}
          />
          Queued Message{queue.length !== 1 ? 's' : ''}
        </Typography>
        {onClearQueue && (
          <IconButton size="small" onClick={onClearQueue} title="Clear all">
            <CloseIcon fontSize="small" />
          </IconButton>
        )}
      </div>
      {queue.map((msg, index) => (
        <Paper key={index} className={classes.queuedMessage} elevation={0}>
          <Chip
            label={`#${index + 1}`}
            size="small"
            variant="outlined"
            sx={{ height: 20, fontSize: '0.75rem' }}
          />
          <span className={classes.messageText}>{msg}</span>
          <div className={classes.messageActions}>
            {onSendNow && (
              <IconButton
                size="small"
                onClick={() => onSendNow(index)}
                title="Send now"
                color="primary"
              >
                <SendIcon fontSize="small" />
              </IconButton>
            )}
            {onDeleteMessage && (
              <IconButton
                size="small"
                onClick={() => onDeleteMessage(index)}
                title="Delete"
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            )}
          </div>
        </Paper>
      ))}
    </Box>
  )
}

QueuedMessages.propTypes = {
  queue: PropTypes.arrayOf(PropTypes.string),
  onSendNow: PropTypes.func,
  onDeleteMessage: PropTypes.func,
  onClearQueue: PropTypes.func,
}
