import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Chip,
  CircularProgress,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  Build as BuildIcon,
  ExpandMore as ExpandMoreIcon,
  Psychology as PsychologyIcon,
  Visibility as VisibilityIcon,
} from '@mui/icons-material'
import { formatChatTimestamp } from './chatUtils'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  thinkingStep: {
    marginBottom: theme.spacing(1),
    overflow: 'hidden', // Prevent accordion from overflowing
    '& .MuiAccordionSummary-root': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(255, 255, 255, 0.05)'
          : 'rgba(0, 0, 0, 0.02)',
      borderRadius: theme.shape.borderRadius,
      minHeight: 'auto', // Allow summary to be compact
    },
    '& .MuiAccordionDetails-root': {
      paddingTop: theme.spacing(1),
    },
  },
  stepHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    flex: 1,
    minWidth: 0,
    width: '100%',
  },
  stepNumber: {
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
    minWidth: 28, // Increased from 24 to accommodate larger numbers
    width: 28,
    height: 28, // Increased from 24
    borderRadius: '50%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '0.75rem',
    fontWeight: 'bold',
    flexShrink: 0, // Prevent the step number from shrinking
  },
  thoughtText: {
    flex: 1,
    minWidth: 0,
    overflow: 'hidden',
    // Remove nowrap to allow text wrapping in collapsed state
    display: '-webkit-box',
    WebkitLineClamp: 2, // Show max 2 lines when collapsed
    WebkitBoxOrient: 'vertical',
    wordBreak: 'break-word', // Break long words
    maxWidth: '100%',
  },
  actionChip: {
    marginLeft: theme.spacing(1),
    fontSize: '0.75rem',
    flexShrink: 0, // Prevent chip from shrinking
  },
  detailSection: {
    marginBottom: theme.spacing(2),
    '&:last-child': {
      marginBottom: 0,
    },
  },
  detailLabel: {
    fontWeight: 'bold',
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(0.5),
  },
  detailContent: {
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    padding: theme.spacing(1),
    borderRadius: theme.shape.borderRadius,
    fontFamily: 'monospace',
    fontSize: '0.875rem',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  },
  inProgress: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    color: theme.palette.primary.main,
  },
  timestamp: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    marginLeft: 'auto',
    flexShrink: 0, // Prevent timestamp from shrinking
    whiteSpace: 'nowrap',
  },
}))

export default function ThinkingStep({
  data,
  isInProgress = false,
  defaultExpanded = false,
}) {
  const classes = useStyles()
  const [expanded, setExpanded] = useState(defaultExpanded || isInProgress)

  const handleExpandChange = (event, isExpanded) => {
    setExpanded(isExpanded)
  }

  const formatTimestamp = (timestamp) => {
    if (!timestamp) return null
    const formatted = formatChatTimestamp(timestamp)
    return (
      <Tooltip title={formatted.relative} arrow>
        <span className={classes.timestamp}>{formatted.main}</span>
      </Tooltip>
    )
  }

  const getActionIcon = (action) => {
    const iconMap = {
      get_prow_job_summary: <BuildIcon fontSize="small" />,
      analyze_job_logs: <VisibilityIcon fontSize="small" />,
      check_known_incidents: <PsychologyIcon fontSize="small" />,
      get_release_payloads: <BuildIcon fontSize="small" />,
      get_payload_details: <VisibilityIcon fontSize="small" />,
    }
    return iconMap[action] || <BuildIcon fontSize="small" />
  }

  const getActionColor = (action) => {
    const colorMap = {
      get_prow_job_summary: 'primary',
      analyze_job_logs: 'secondary',
      check_known_incidents: 'warning',
      get_release_payloads: 'info',
      get_payload_details: 'success',
    }
    return colorMap[action] || 'default'
  }

  const renderSummary = () => (
    <div className={classes.stepHeader}>
      <div className={classes.stepNumber}>
        {isInProgress ? (
          <CircularProgress size={16} color="inherit" />
        ) : (
          data.step_number || '?'
        )}
      </div>

      <PsychologyIcon color="action" fontSize="small" />

      <Typography variant="body2" className={classes.thoughtText}>
        {isInProgress ? 'Thinking...' : data.thought || 'Processing...'}
      </Typography>

      {data.action && (
        <Chip
          icon={getActionIcon(data.action)}
          label={data.action.replace(/_/g, ' ')}
          size="small"
          color={getActionColor(data.action)}
          variant="outlined"
          className={classes.actionChip}
        />
      )}

      {data.timestamp && formatTimestamp(data.timestamp)}
    </div>
  )

  const renderDetails = () => (
    <Box>
      {data.thought && (
        <div className={classes.detailSection}>
          <Typography variant="subtitle2" className={classes.detailLabel}>
            Thought Process
          </Typography>
          <div className={classes.detailContent}>{data.thought}</div>
        </div>
      )}

      {data.action && (
        <div className={classes.detailSection}>
          <Typography variant="subtitle2" className={classes.detailLabel}>
            Action
          </Typography>
          <div className={classes.detailContent}>{data.action}</div>
        </div>
      )}

      {data.action_input && (
        <div className={classes.detailSection}>
          <Typography variant="subtitle2" className={classes.detailLabel}>
            Action Input
          </Typography>
          <div className={classes.detailContent}>
            {typeof data.action_input === 'string'
              ? data.action_input
              : JSON.stringify(data.action_input, null, 2)}
          </div>
        </div>
      )}

      {data.observation && (
        <div className={classes.detailSection}>
          <Typography variant="subtitle2" className={classes.detailLabel}>
            Observation
          </Typography>
          <div className={classes.detailContent}>{data.observation}</div>
        </div>
      )}

      {isInProgress && (
        <div className={classes.inProgress}>
          <CircularProgress size={16} />
          <Typography variant="body2">Executing action...</Typography>
        </div>
      )}
    </Box>
  )

  return (
    <Accordion
      expanded={expanded}
      onChange={handleExpandChange}
      className={classes.thinkingStep}
      elevation={1}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        {renderSummary()}
      </AccordionSummary>
      <AccordionDetails>{renderDetails()}</AccordionDetails>
    </Accordion>
  )
}

ThinkingStep.propTypes = {
  data: PropTypes.shape({
    step_number: PropTypes.number,
    thought: PropTypes.string,
    action: PropTypes.string,
    action_input: PropTypes.oneOfType([PropTypes.string, PropTypes.object]),
    observation: PropTypes.string,
    complete: PropTypes.bool,
    timestamp: PropTypes.string,
    iteration: PropTypes.number,
  }).isRequired,
  isInProgress: PropTypes.bool,
  defaultExpanded: PropTypes.bool,
}
