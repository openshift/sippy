import { Alert, Avatar, Chip, IconButton, Paper, Tooltip } from '@mui/material'
import {
  ContentCopy as ContentCopyIcon,
  Error as ErrorIcon,
  Link as LinkIcon,
  OpenInNew as OpenInNewIcon,
  Person as PersonIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material'
import { formatChatTimestamp, humanize, MESSAGE_TYPES } from './chatUtils'
import { Link } from 'react-router-dom'
import { makeStyles, useTheme } from '@mui/styles'
import { useModels } from './store/useChatStore'
import MessageChart from './MessageChart'
import PropTypes from 'prop-types'
import React from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

// Custom link component for ReactMarkdown that opens external links in new tabs
const ChatLink = ({ href, children, ...props }) => {
  const theme = useTheme()
  const isExternal =
    href && (href.startsWith('http://') || href.startsWith('https://'))

  // Let CSS handle the styling - just ensure proper attributes
  if (isExternal) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
        {children}
      </a>
    )
  }

  // Internal links or non-http links stay in same tab
  return (
    <a href={href} {...props}>
      {children}
    </a>
  )
}

ChatLink.propTypes = {
  href: PropTypes.string,
  children: PropTypes.node,
}

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
  aiChip: {
    height: 18,
    fontSize: '0.65rem',
    fontWeight: 600,
    '& .MuiChip-label': {
      padding: '0 6px',
    },
  },
  markdownContent: {
    minWidth: 0, // Allow markdown content to shrink
    overflow: 'hidden', // Prevent overflow
    wordBreak: 'break-word', // Break long words
    whiteSpace: 'normal', // Override pre-wrap from messageText
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
      paddingLeft: 20, // Standard padding for proper bullet alignment
      marginLeft: 0, // Ensure no extra left margin
    },
    '& li': {
      margin: '2px 0', // Reduced from 4px for tighter spacing
      paddingLeft: 0, // Remove any extra padding on list items
      wordBreak: 'normal', // Override the global word-break to prevent text wrapping issues
      display: 'list-item', // Ensure proper list item display
      lineHeight: 1.5, // Consistent line height for better alignment
      '& p': {
        display: 'inline', // Make paragraphs within list items inline instead of block
        margin: 0, // Remove default paragraph margins
        padding: 0, // Remove default paragraph padding
      },
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
      color: theme.palette.mode === 'dark' ? '#64b5f6' : '#1976d2', // Light blue for dark mode, darker blue for light mode
      textDecoration: 'underline',
      cursor: 'pointer',
      fontWeight: 500, // Make links slightly bolder
      '&:hover': {
        textDecoration: 'underline',
        opacity: 0.8,
        backgroundColor:
          theme.palette.mode === 'dark'
            ? 'rgba(100, 181, 246, 0.1)'
            : 'rgba(25, 118, 210, 0.1)', // Subtle background on hover
      },
      '&:visited': {
        color: theme.palette.mode === 'dark' ? '#ba68c8' : '#7b1fa2', // Purple for visited links
      },
    },
    '& table': {
      borderCollapse: 'collapse',
      width: '100%',
      margin: '8px 0',
      fontSize: '0.875rem',
      overflow: 'auto',
      display: 'block',
      maxWidth: '100%',
    },
    '& thead': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(255, 255, 255, 0.1)'
          : 'rgba(0, 0, 0, 0.05)',
    },
    '& th': {
      border: `1px solid ${theme.palette.divider}`,
      padding: '8px 12px',
      textAlign: 'left',
      fontWeight: 600,
    },
    '& td': {
      border: `1px solid ${theme.palette.divider}`,
      padding: '8px 12px',
      textAlign: 'left',
    },
    '& tr:nth-of-type(even)': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(255, 255, 255, 0.02)'
          : 'rgba(0, 0, 0, 0.02)',
    },
  },
  systemMessage: {
    textAlign: 'center',
    padding: theme.spacing(2, 0),
    margin: theme.spacing(2, 0),
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
  },
  systemMessageText: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    fontStyle: 'italic',
    marginBottom: theme.spacing(1),
  },
  systemMessageDivider: {
    width: '60%',
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
}))

export default function ChatMessage({
  message,
  showTimestamp = true,
  showTools = true,
}) {
  const classes = useStyles()
  const { models } = useModels()

  const handleCopyMessage = async () => {
    try {
      await navigator.clipboard.writeText(message.content)
      // Could add a toast notification here
    } catch (err) {
      console.error('Failed to copy message:', err)
    }
  }

  // Get model name for display
  const getModelName = () => {
    if (!message.model_id || models.length === 0) {
      return null
    }
    const model = models.find((m) => m.id === message.model_id)
    return model ? model.name : null
  }

  const modelName = getModelName()

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
            <ReactMarkdown
              components={{ a: ChatLink }}
              remarkPlugins={[remarkGfm]}
            >
              {message.content}
            </ReactMarkdown>
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
            <ReactMarkdown
              components={{ a: ChatLink }}
              remarkPlugins={[remarkGfm]}
            >
              {message.content}
            </ReactMarkdown>
          </div>

          <MessageChart visualizations={message.visualizations} />

          <div className={classes.messageFooter}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              {formatTimestamp(message.timestamp)}
              <Tooltip
                title={
                  modelName
                    ? `AI-generated by ${modelName}`
                    : 'AI-generated response'
                }
                arrow
              >
                <Chip
                  label={modelName ? modelName : 'AI'}
                  size="small"
                  className={classes.aiChip}
                  color="secondary"
                  variant="outlined"
                />
              </Tooltip>
            </div>
            <div style={{ display: 'flex', gap: 4 }}>
              {message.pageContext &&
                message.pageContext.url &&
                message.pageContext.page && (
                  <Tooltip title={`View ${humanize(message.pageContext.page)}`}>
                    <IconButton
                      size="small"
                      component={Link}
                      to={message.pageContext.url}
                      className={classes.copyButton}
                    >
                      <OpenInNewIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                )}
              <IconButton
                size="small"
                onClick={handleCopyMessage}
                className={classes.copyButton}
                title="Copy message"
              >
                <ContentCopyIcon fontSize="small" />
              </IconButton>
            </div>
          </div>
        </Paper>
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
          <ReactMarkdown
            components={{ a: ChatLink }}
            remarkPlugins={[remarkGfm]}
          >
            {message.content}
          </ReactMarkdown>
        </div>
        {formatTimestamp(message.timestamp)}
      </Alert>
    </div>
  )

  const renderSystemMessage = () => (
    <div className={classes.systemMessage}>
      <div className={classes.systemMessageText}>
        {message.content}
        {message.conversationId && (
          <>
            {' '}
            <Link
              to={`/chat/${message.conversationId}`}
              style={{
                color: 'inherit',
                textDecoration: 'none',
                display: 'inline-flex',
                alignItems: 'center',
                verticalAlign: 'middle',
              }}
            >
              <LinkIcon fontSize="small" />
            </Link>
          </>
        )}
      </div>
      <div className={classes.systemMessageDivider} />
    </div>
  )

  // Render based on message type
  switch (message.type) {
    case MESSAGE_TYPES.USER:
      return renderUserMessage()

    case MESSAGE_TYPES.ASSISTANT:
      return renderAssistantMessage()

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
    model_id: PropTypes.string,
    visualizations: PropTypes.arrayOf(
      PropTypes.shape({
        data: PropTypes.array.isRequired,
        layout: PropTypes.object.isRequired,
        config: PropTypes.object,
      })
    ),
    conversationId: PropTypes.string,
    pageContext: PropTypes.shape({
      page: PropTypes.string,
      url: PropTypes.string,
      data: PropTypes.object,
    }),
  }).isRequired,
  showTimestamp: PropTypes.bool,
  showTools: PropTypes.bool,
}
