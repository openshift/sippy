import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Divider,
  Drawer,
  FormControl,
  IconButton,
  InputLabel,
  List,
  ListItem,
  ListItemSecondaryAction,
  ListItemText,
  MenuItem,
  Select,
  Switch,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  VerticalAlignBottom as AutoScrollIcon,
  Close as CloseIcon,
  Delete as DeleteIcon,
  Info as InfoIcon,
  Masks as MasksIcon,
  Memory as ModelIcon,
  Psychology as PsychologyIcon,
  Refresh as RefreshIcon,
  Settings as SettingsIcon,
  Storage as StorageIcon,
  TravelExplore as TourIcon,
} from '@mui/icons-material'
import { CONNECTION_STATES } from './store/webSocketSlice'
import { formatBytes, getChatStorageStats } from './store/storageUtils'
import { humanize } from './chatUtils'
import { makeStyles } from '@mui/styles'
import {
  useConnectionState,
  useModels,
  usePersonas,
  useSessionActions,
  useSessionState,
  useSettings,
} from './store/useChatStore'
import PropTypes from 'prop-types'
import React, { useCallback, useEffect, useState } from 'react'

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
    marginTop: theme.spacing(2),
  },
  sectionTitle: {
    marginBottom: theme.spacing(1),
    fontWeight: 'bold',
  },
  settingItem: {
    paddingLeft: 0,
    paddingRight: 0,
    alignItems: 'center',
  },
  settingItemAction: {
    top: 20,
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
  personaSelect: {
    width: '100%',
  },
  personaDescription: {
    marginTop: theme.spacing(1),
    marginBottom: theme.spacing(1),
    padding: theme.spacing(1),
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    borderRadius: theme.shape.borderRadius,
  },
}))

export default function ChatSettings({ onClearMessages, onReconnect }) {
  const classes = useStyles()

  const { settings, settingsOpen, updateSettings, setSettingsOpen, resetTour } =
    useSettings()
  const { connectionState } = useConnectionState()
  const { personas, personasLoading, personasError, loadPersonas } =
    usePersonas()
  const { models, defaultModel, modelsLoading, modelsError, loadModels } =
    useModels()
  const { sessions, activeSessionId } = useSessionState()
  const { clearAllSessions, clearOldSessions } = useSessionActions()

  const isConnected = connectionState === 'connected'
  const tourCompleted = settings.tourCompleted

  // Storage stats
  const [storageStats, setStorageStats] = useState({
    conversationCount: 0,
    sizeBytes: 0,
  })
  const [storageLoading, setStorageLoading] = useState(false)

  useEffect(() => {
    if (personas.length === 0 && !personasLoading) {
      loadPersonas()
    }
  }, [personas.length, personasLoading, loadPersonas])

  useEffect(() => {
    if (models.length === 0 && !modelsLoading) {
      loadModels()
    }
  }, [models.length, modelsLoading, loadModels])

  // Load storage stats
  const loadStorageStats = useCallback(async () => {
    setStorageLoading(true)
    try {
      const stats = await getChatStorageStats(
        sessions,
        activeSessionId,
        settings
      )
      setStorageStats(stats)
    } catch (error) {
      console.error('Error loading storage stats:', error)
    } finally {
      setStorageLoading(false)
    }
  }, [sessions, activeSessionId, settings])

  // Load storage stats when drawer opens or sessions change
  useEffect(() => {
    if (open) {
      loadStorageStats()
    }
  }, [open, loadStorageStats])

  const handleSettingChange = (key) => (event) => {
    updateSettings({
      [key]: event.target.checked,
    })
  }

  const handlePersonaChange = (event) => {
    updateSettings({
      persona: event.target.value,
    })
  }

  const handleModelChange = (event) => {
    updateSettings({
      modelId: event.target.value,
    })
  }

  const getSelectedPersona = () => {
    return personas.find((p) => p.name === settings.persona) || personas[0]
  }

  const getSelectedModel = () => {
    return models.find((m) => m.id === settings.modelId) || models[0]
  }

  const getConnectionStatusText = () => {
    switch (connectionState) {
      case CONNECTION_STATES.CONNECTING:
        return 'Connecting...'
      case CONNECTION_STATES.CONNECTED:
        return 'Connected'
      case CONNECTION_STATES.DISCONNECTED:
        return 'Disconnected'
      default:
        return 'Unknown'
    }
  }

  const getConnectionStatusColor = () => {
    switch (connectionState) {
      case CONNECTION_STATES.CONNECTED:
        return 'success'
      case CONNECTION_STATES.CONNECTING:
        return 'warning'
      case CONNECTION_STATES.DISCONNECTED:
        return 'error'
      default:
        return 'info'
    }
  }

  const handleClearOldConversations = async () => {
    const clearedCount = clearOldSessions(1) // Clear conversations older than 1 day
    await loadStorageStats()
    if (clearedCount > 0) {
      console.log(`Cleared ${clearedCount} old conversation(s)`)
    }
  }

  const handleClearAllConversations = async () => {
    if (
      window.confirm(
        'Are you sure you want to clear all saved conversations? This cannot be undone.'
      )
    ) {
      clearAllSessions()
      await loadStorageStats()
      onClearMessages() // Also clear current messages
    }
  }

  const handleRestartTour = () => {
    resetTour()
    setSettingsOpen(false)
  }

  return (
    <Drawer
      className={classes.drawer}
      variant="temporary"
      anchor="right"
      open={settingsOpen}
      onClose={() => setSettingsOpen(false)}
      classes={{
        paper: classes.drawerPaper,
      }}
    >
      <div className={classes.header}>
        <Box display="flex" alignItems="center" gap={1}>
          <SettingsIcon />
          <Typography variant="h6">Chat Settings</Typography>
        </Box>
        <IconButton onClick={() => setSettingsOpen(false)} size="small">
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

      <Divider />

      {/* Persona Selection */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          AI Persona
        </Typography>

        {personasLoading ? (
          <Box display="flex" alignItems="center" gap={1} mb={2}>
            <CircularProgress size={20} />
            <Typography variant="body2" color="textSecondary">
              Loading personas...
            </Typography>
          </Box>
        ) : personasError ? (
          <Alert severity="warning" sx={{ mb: 2 }}>
            Could not load personas. Using default.
          </Alert>
        ) : (
          <>
            <FormControl className={classes.personaSelect}>
              <InputLabel id="persona-select-label">Select Persona</InputLabel>
              <Select
                labelId="persona-select-label"
                value={settings.persona || 'default'}
                onChange={handlePersonaChange}
                label="Select Persona"
                startAdornment={<MasksIcon sx={{ mr: 1 }} />}
              >
                {personas.map((persona) => (
                  <MenuItem key={persona.name} value={persona.name}>
                    {humanize(persona.name)}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            {getSelectedPersona() && (
              <Box className={classes.personaDescription}>
                <Typography variant="body2" color="textPrimary" gutterBottom>
                  {getSelectedPersona().description}
                </Typography>
                {getSelectedPersona().style_instructions && (
                  <Typography variant="caption" color="textSecondary">
                    {getSelectedPersona().style_instructions}
                  </Typography>
                )}
              </Box>
            )}
          </>
        )}
      </div>

      <Divider />

      {/* Model Selection */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          AI Model
        </Typography>

        {modelsLoading ? (
          <Box display="flex" alignItems="center" gap={1} mb={2}>
            <CircularProgress size={20} />
            <Typography variant="body2" color="textSecondary">
              Loading models...
            </Typography>
          </Box>
        ) : modelsError ? (
          <Alert severity="warning" sx={{ mb: 2 }}>
            Could not load models. Using default.
          </Alert>
        ) : (
          <>
            <FormControl className={classes.personaSelect}>
              <InputLabel id="model-select-label">Select Model</InputLabel>
              <Select
                labelId="model-select-label"
                value={settings.modelId || defaultModel || ''}
                onChange={handleModelChange}
                label="Select Model"
                startAdornment={<ModelIcon sx={{ mr: 1 }} />}
              >
                {models.map((model) => (
                  <MenuItem key={model.id} value={model.id}>
                    {model.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            {getSelectedModel() && getSelectedModel().description && (
              <Box className={classes.personaDescription}>
                <Typography variant="body2" color="textPrimary">
                  {getSelectedModel().description}
                </Typography>
              </Box>
            )}
          </>
        )}
      </div>

      <Divider />

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
              primaryTypographyProps={{ variant: 'body2' }}
              secondaryTypographyProps={{ variant: 'caption' }}
            />
            <ListItemSecondaryAction className={classes.settingItemAction}>
              <Switch
                edge="end"
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
              primaryTypographyProps={{ variant: 'body2' }}
              secondaryTypographyProps={{ variant: 'caption' }}
            />
            <ListItemSecondaryAction className={classes.settingItemAction}>
              <Switch
                edge="end"
                checked={settings.autoScroll}
                onChange={handleSettingChange('autoScroll')}
                color="primary"
                icon={<AutoScrollIcon />}
                checkedIcon={<AutoScrollIcon />}
              />
            </ListItemSecondaryAction>
          </ListItem>
        </List>
      </div>

      <Divider />

      {/* Storage Management */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          <Box display="flex" alignItems="center" gap={0.5}>
            <StorageIcon fontSize="small" />
            Storage
            <Tooltip
              title="Conversations are stored locally in your browser. Only shared conversations are persisted server-side."
              placement="top"
            >
              <InfoIcon
                fontSize="small"
                sx={{ color: 'text.secondary', cursor: 'help' }}
              />
            </Tooltip>
          </Box>
        </Typography>

        {storageLoading ? (
          <Box display="flex" alignItems="center" gap={1} mb={2}>
            <CircularProgress size={20} />
            <Typography variant="body2" color="textSecondary">
              Loading storage info...
            </Typography>
          </Box>
        ) : (
          <>
            <Box
              sx={{
                mb: 2,
                p: 1.5,
                backgroundColor: (theme) =>
                  theme.palette.mode === 'dark'
                    ? 'rgba(255, 255, 255, 0.05)'
                    : 'rgba(0, 0, 0, 0.02)',
                borderRadius: 1,
              }}
            >
              <Box
                display="flex"
                justifyContent="space-between"
                alignItems="center"
                mb={1}
              >
                <Typography variant="body2" color="textSecondary">
                  Conversations:
                </Typography>
                <Typography variant="body2" fontWeight="bold">
                  {sessions.length}
                </Typography>
              </Box>
              <Box
                display="flex"
                justifyContent="space-between"
                alignItems="center"
              >
                <Typography variant="body2" color="textSecondary">
                  Storage used:
                </Typography>
                <Typography variant="body2" fontWeight="bold">
                  {formatBytes(storageStats.sizeBytes)}
                </Typography>
              </Box>
            </Box>

            <Box display="flex" flexDirection="column" gap={1}>
              <Button
                variant="outlined"
                startIcon={<DeleteIcon />}
                onClick={handleClearOldConversations}
                fullWidth
                size="small"
              >
                Clear Old (1+ day)
              </Button>
              <Button
                variant="outlined"
                startIcon={<DeleteIcon />}
                onClick={handleClearAllConversations}
                className={classes.dangerButton}
                fullWidth
                size="small"
              >
                Clear All Conversations
              </Button>
            </Box>
          </>
        )}
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

      <Divider />

      {/* Tour Management */}
      <div className={classes.section}>
        <Typography variant="subtitle2" className={classes.sectionTitle}>
          Help & Guidance
        </Typography>

        <Button
          variant="outlined"
          startIcon={<TourIcon />}
          onClick={handleRestartTour}
          fullWidth
          disabled={!tourCompleted}
        >
          Restart Interface Tour
        </Button>
        {!tourCompleted && (
          <Typography
            variant="caption"
            color="textSecondary"
            sx={{ display: 'block', mt: 1, textAlign: 'center' }}
          >
            The tour will start automatically on first use
          </Typography>
        )}
      </div>
    </Drawer>
  )
}

ChatSettings.propTypes = {
  onClearMessages: PropTypes.func.isRequired,
  onReconnect: PropTypes.func.isRequired,
}
