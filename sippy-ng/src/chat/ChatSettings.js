import {
  Alert,
  Box,
  Button,
  Divider,
  Drawer,
  IconButton,
  List,
  ListItem,
  ListItemSecondaryAction,
  ListItemText,
  Switch,
  Typography,
} from '@mui/material'
import {
  VerticalAlignBottom as AutoScrollIcon,
  Close as CloseIcon,
  Delete as DeleteIcon,
  Psychology as PsychologyIcon,
  Refresh as RefreshIcon,
  Settings as SettingsIcon,
} from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles((theme) => ({
  drawer: {
    width: 320,
    flexShrink: 0,
  },
  drawerPaper: {
    width: 320,
    padding: theme.spacing(2),
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: theme.spacing(2),
  },
  section: {
    marginBottom: theme.spacing(3),
  },
  sectionTitle: {
    marginBottom: theme.spacing(1),
    fontWeight: 'bold',
  },
  settingItem: {
    paddingLeft: 0,
    paddingRight: 0,
  },
  dangerButton: {
    color: theme.palette.error.main,
    borderColor: theme.palette.error.main,
    '&:hover': {
      backgroundColor: theme.palette.error.main,
      color: theme.palette.error.contrastText,
    },
  },
  connectionInfo: {
    marginBottom: theme.spacing(2),
  },
}))

export default function ChatSettings({
  open,
  onClose,
  settings,
  onSettingsChange,
  onClearMessages,
  onReconnect,
  connectionState,
  messageCount,
  isConnected,
}) {
  const classes = useStyles()

  const handleSettingChange = (key) => (event) => {
    onSettingsChange({
      ...settings,
      [key]: event.target.checked,
    })
  }

  const getConnectionStatusText = () => {
    switch (connectionState) {
      case 0:
        return 'Connecting...'
      case 1:
        return 'Connected'
      case 2:
        return 'Disconnecting...'
      case 3:
        return 'Disconnected'
      default:
        return 'Unknown'
    }
  }

  const getConnectionStatusColor = () => {
    switch (connectionState) {
      case 1:
        return 'success'
      case 0:
      case 2:
        return 'warning'
      case 3:
        return 'error'
      default:
        return 'info'
    }
  }

  return (
    <Drawer
      className={classes.drawer}
      variant="temporary"
      anchor="right"
      open={open}
      onClose={onClose}
      classes={{
        paper: classes.drawerPaper,
      }}
    >
      <div className={classes.header}>
        <Box display="flex" alignItems="center" gap={1}>
          <SettingsIcon />
          <Typography variant="h6">Chat Settings</Typography>
        </Box>
        <IconButton onClick={onClose} size="small">
          <CloseIcon />
        </IconButton>
      </div>

      {/* Connection Status */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          Connection Status
        </Typography>
        <Alert
          severity={getConnectionStatusColor()}
          className={classes.connectionInfo}
          action={
            !isConnected && (
              <IconButton size="small" onClick={onReconnect}>
                <RefreshIcon fontSize="small" />
              </IconButton>
            )
          }
        >
          {getConnectionStatusText()}
        </Alert>
      </div>

      {/* Display Settings */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          Display Options
        </Typography>
        <List dense>
          <ListItem className={classes.settingItem}>
            <ListItemText
              primary="Show Thinking Steps"
              secondary="Display the agent's reasoning process"
            />
            <ListItemSecondaryAction>
              <Switch
                checked={settings.showThinking}
                onChange={handleSettingChange('showThinking')}
                color="primary"
                icon={<PsychologyIcon />}
                checkedIcon={<PsychologyIcon />}
              />
            </ListItemSecondaryAction>
          </ListItem>

          <ListItem className={classes.settingItem}>
            <ListItemText
              primary="Auto Scroll"
              secondary="Automatically scroll to new messages"
            />
            <ListItemSecondaryAction>
              <Switch
                checked={settings.autoScroll}
                onChange={handleSettingChange('autoScroll')}
                color="primary"
                icon={<AutoScrollIcon />}
                checkedIcon={<AutoScrollIcon />}
              />
            </ListItemSecondaryAction>
          </ListItem>

          <ListItem className={classes.settingItem}>
            <ListItemText
              primary="Retry Failed Messages"
              secondary="Automatically retry failed messages"
            />
            <ListItemSecondaryAction>
              <Switch
                checked={settings.retryFailedMessages}
                onChange={handleSettingChange('retryFailedMessages')}
                color="primary"
              />
            </ListItemSecondaryAction>
          </ListItem>
        </List>
      </div>

      <Divider />

      {/* Chat Management */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          Chat Management
        </Typography>

        <Typography variant="body2" color="textSecondary" gutterBottom>
          Messages in conversation: {messageCount}
        </Typography>

        <Button
          variant="outlined"
          startIcon={<DeleteIcon />}
          onClick={onClearMessages}
          className={classes.dangerButton}
          fullWidth
          disabled={messageCount === 0}
        >
          Clear Chat History
        </Button>
      </div>

      <Divider />

      {/* Connection Management */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          Connection
        </Typography>

        <Button
          variant="outlined"
          startIcon={<RefreshIcon />}
          onClick={onReconnect}
          fullWidth
          disabled={isConnected}
        >
          Reconnect
        </Button>
      </div>

      {/* Help Text */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          Tips
        </Typography>
        <Typography variant="body2" color="textSecondary">
          • Ask about specific job IDs for detailed analysis
          <br />
          • Request payload status by version number
          <br />
          • Inquire about test failure patterns
          <br />• Use Shift+Enter for new lines in messages
        </Typography>
      </div>
    </Drawer>
  )
}

ChatSettings.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  settings: PropTypes.shape({
    showThinking: PropTypes.bool,
    autoScroll: PropTypes.bool,
    retryFailedMessages: PropTypes.bool,
  }).isRequired,
  onSettingsChange: PropTypes.func.isRequired,
  onClearMessages: PropTypes.func.isRequired,
  onReconnect: PropTypes.func.isRequired,
  connectionState: PropTypes.number.isRequired,
  messageCount: PropTypes.number.isRequired,
  isConnected: PropTypes.bool.isRequired,
}
