import { Alert, Avatar, Chip, IconButton, Paper, Tooltip } from '@mui/material'
import {
  ContentCopy as ContentCopyIcon,
  Error as ErrorIcon,
  Info as InfoIcon,
  Person as PersonIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material'
import { formatChatTimestamp, MESSAGE_TYPES } from './chatUtils'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React from 'react'
import ReactMarkdown from 'react-markdown'
import ThinkingStep from './ThinkingStep'

const useStyles = makeStyles((theme) => ({
  messageContainer: {
    display: 'flex',
    marginBottom: theme.spacing(2),
    '&.user': {
      justifyContent: 'flex-end',
    },
    '&.assistant': {
      justifyContent: 'flex-start',
    },
    '&.system': {
      justifyContent: 'center',
    },
  },
  messageContent: {
    maxWidth: '70%',
    display: 'flex',
    gap: theme.spacing(1),
    minWidth: 0, // Allow content to shrink
    '&.user': {
      flexDirection: 'row-reverse',
    },
  },
  messagePaper: {
    padding: theme.spacing(1.5),
    position: 'relative',
    minWidth: 0, // Allow paper to shrink
    overflow: 'hidden', // Prevent content overflow
    wordBreak: 'break-word', // Break long words
    '&.user': {
      backgroundColor: theme.palette.primary.main,
      color: theme.palette.primary.contrastText,
      borderBottomRightRadius: 4,
    },
    '&.assistant': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? theme.palette.grey[800]
          : theme.palette.grey[100],
      borderBottomLeftRadius: 4,
    },
    '&.error': {
      backgroundColor: theme.palette.error.main,
      color: theme.palette.error.contrastText,
    },
  },
  messageText: {
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
    marginBottom: theme.spacing(0.5),
    minWidth: 0, // Allow text to shrink
    overflow: 'hidden', // Prevent overflow
  },
  messageFooter: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: theme.spacing(1),
    gap: theme.spacing(1),
  },
  timestamp: {
    fontSize: '0.75rem',
    opacity: 0.7,
  },
  toolsUsed: {
    display: 'flex',
    gap: theme.spacing(0.5),
    flexWrap: 'wrap',
  },
  toolChip: {
    fontSize: '0.7rem',
    height: 20,
  },
  avatar: {
    width: 32,
    height: 32,
    '&.user': {
      backgroundColor: theme.palette.primary.main,
    },
    '&.assistant': {
      backgroundColor: theme.palette.secondary.main,
    },
  },
  copyButton: {
    padding: 4,
    opacity: 0.7,
    '&:hover': {
      opacity: 1,
    },
  },
  markdownContent: {
    minWidth: 0, // Allow markdown content to shrink
    overflow: 'hidden', // Prevent overflow
    wordBreak: 'break-word', // Break long words
    '& p': {
      margin: '0 0 8px 0',
      wordBreak: 'break-word', // Break long words in paragraphs
      '&:last-child': {
        marginBottom: 0,
      },
    },
    '& h1, & h2, & h3, & h4, & h5, & h6': {
      margin: '16px 0 8px 0',
      '&:first-child': {
        marginTop: 0,
      },
    },
    '& ul, & ol': {
      margin: '8px 0',
      paddingLeft: 20,
    },
    '& li': {
      margin: '4px 0',
    },
    '& code': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(255, 255, 255, 0.1)'
          : 'rgba(0, 0, 0, 0.1)',
      padding: '2px 4px',
      borderRadius: 4,
      fontFamily: 'monospace',
      fontSize: '0.875em',
    },
    '& pre': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(255, 255, 255, 0.05)'
          : 'rgba(0, 0, 0, 0.05)',
      padding: theme.spacing(1),
      borderRadius: 4,
      overflow: 'auto',
      margin: '8px 0',
      maxWidth: '100%', // Prevent pre blocks from overflowing
      wordBreak: 'break-all', // Break long lines in code blocks
      '& code': {
        backgroundColor: 'transparent',
        padding: 0,
        wordBreak: 'break-all', // Break long code lines
      },
    },
    '& blockquote': {
      borderLeft: `4px solid ${theme.palette.primary.main}`,
      paddingLeft: theme.spacing(1),
      margin: '8px 0',
      fontStyle: 'italic',
    },
    '& a': {
      color: theme.palette.primary.main,
      textDecoration: 'none',
      '&:hover': {
        textDecoration: 'underline',
      },
    },
  },
  systemMessage: {
    maxWidth: '80%',
  },
  thinkingContainer: {
    width: '100%',
    minWidth: 0, // Allow container to shrink
    overflow: 'hidden', // Prevent overflow
    marginLeft: theme.spacing(5), // Add left margin to align with other assistant messages
  },
}))

export default function ChatMessage({
  message,
  showTimestamp = true,
  showTools = true,
}) {
  const classes = useStyles()

  const handleCopyMessage = async () => {
    try {
      await navigator.clipboard.writeText(message.content)
      // Could add a toast notification here
    } catch (err) {
      console.error('Failed to copy message:', err)
    }
  }

  const formatTimestamp = (timestamp) => {
    if (!timestamp || !showTimestamp) return null
    const formatted = formatChatTimestamp(timestamp)
    return (
      <Tooltip title={formatted.relative} arrow>
        <span className={classes.timestamp}>{formatted.main}</span>
      </Tooltip>
    )
  }

  const renderUserMessage = () => (
    <div className={`${classes.messageContainer} user`}>
      <div className={`${classes.messageContent} user`}>
        <Avatar className={`${classes.avatar} user`}>
          <PersonIcon />
        </Avatar>
        <Paper className={`${classes.messagePaper} user`} elevation={2}>
          <div className={`${classes.messageText} ${classes.markdownContent}`}>
            <ReactMarkdown>{message.content}</ReactMarkdown>
          </div>
          <div className={classes.messageFooter}>
            {formatTimestamp(message.timestamp)}
            <IconButton
              size="small"
              onClick={handleCopyMessage}
              className={classes.copyButton}
              title="Copy message"
            >
              <ContentCopyIcon fontSize="small" />
            </IconButton>
          </div>
        </Paper>
      </div>
    </div>
  )

  const renderAssistantMessage = () => (
    <div className={`${classes.messageContainer} assistant`}>
      <div className={`${classes.messageContent} assistant`}>
        <Avatar className={`${classes.avatar} assistant`}>
          <SmartToyIcon />
        </Avatar>
        <Paper className={`${classes.messagePaper} assistant`} elevation={2}>
          <div className={`${classes.messageText} ${classes.markdownContent}`}>
            <ReactMarkdown>{message.content}</ReactMarkdown>
          </div>
          <div className={classes.messageFooter}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              {formatTimestamp(message.timestamp)}
              {showTools &&
                message.tools_used &&
                message.tools_used.length > 0 && (
                  <div className={classes.toolsUsed}>
                    {message.tools_used.map((tool, index) => (
                      <Chip
                        key={index}
                        label={tool.replace(/_/g, ' ')}
                        size="small"
                        variant="outlined"
                        className={classes.toolChip}
                      />
                    ))}
                  </div>
                )}
            </div>
            <IconButton
              size="small"
              onClick={handleCopyMessage}
              className={classes.copyButton}
              title="Copy message"
            >
              <ContentCopyIcon fontSize="small" />
            </IconButton>
          </div>
        </Paper>
      </div>
    </div>
  )

  const renderThinkingStep = () => (
    <div className={`${classes.messageContainer} assistant`}>
      <div className={`${classes.messageContent} assistant`}>
        <div className={classes.thinkingContainer}>
          <ThinkingStep
            data={message.data}
            isInProgress={!message.data?.complete}
            defaultExpanded={!message.data?.complete}
          />
        </div>
      </div>
    </div>
  )

  const renderErrorMessage = () => (
    <div className={`${classes.messageContainer} system`}>
      <Alert
        severity="error"
        icon={<ErrorIcon />}
        className={classes.systemMessage}
        action={
          <IconButton
            size="small"
            onClick={handleCopyMessage}
            className={classes.copyButton}
            title="Copy error"
          >
            <ContentCopyIcon fontSize="small" />
          </IconButton>
        }
      >
        <div className={classes.markdownContent}>
          <ReactMarkdown>{message.content}</ReactMarkdown>
        </div>
        {formatTimestamp(message.timestamp)}
      </Alert>
    </div>
  )

  const renderSystemMessage = () => (
    <div className={`${classes.messageContainer} system`}>
      <Alert
        severity="info"
        icon={<InfoIcon />}
        className={classes.systemMessage}
      >
        <div className={classes.markdownContent}>
          <ReactMarkdown>{message.content}</ReactMarkdown>
        </div>
        {formatTimestamp(message.timestamp)}
      </Alert>
    </div>
  )

  // Render based on message type
  switch (message.type) {
    case MESSAGE_TYPES.USER:
      return renderUserMessage()

    case MESSAGE_TYPES.ASSISTANT:
      return renderAssistantMessage()

    case MESSAGE_TYPES.THINKING_STEP:
      return renderThinkingStep()

    case MESSAGE_TYPES.ERROR:
      return renderErrorMessage()

    case MESSAGE_TYPES.SYSTEM:
      return renderSystemMessage()

    default:
      console.warn('Unknown message type:', message.type)
      return null
  }
}

ChatMessage.propTypes = {
  message: PropTypes.shape({
    id: PropTypes.string.isRequired,
    type: PropTypes.string.isRequired,
    content: PropTypes.string.isRequired,
    timestamp: PropTypes.string.isRequired,
    data: PropTypes.object,
    tools_used: PropTypes.arrayOf(PropTypes.string),
  }).isRequired,
  showTimestamp: PropTypes.bool,
  showTools: PropTypes.bool,
}
