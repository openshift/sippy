import { AutoAwesome as AutoAwesomeIcon } from '@mui/icons-material'
import { Button, Tooltip } from '@mui/material'
import { CapabilitiesContext } from '../App'
import { makeStyles } from '@mui/styles'
import {
  useDrawer,
  useSessionActions,
  useWebSocketActions,
} from './store/useChatStore'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'

const useStyles = makeStyles((theme) => ({
  defaultStyledButton: {
    background: 'linear-gradient(45deg, #2196F3 30%, #21CBF3 90%)',
    boxShadow: '0 3px 5px 2px rgba(33, 203, 243, .3)',
    color: 'white',
    fontWeight: 'bold',
    textTransform: 'none',
    transition: 'all 0.3s ease',
    animation: '$pulse 2s ease-in-out infinite',
    '&:hover': {
      background: 'linear-gradient(45deg, #1976D2 30%, #00BCD4 90%)',
      boxShadow: '0 6px 20px 4px rgba(33, 203, 243, .4)',
      transform: 'translateY(-2px)',
    },
  },
  '@keyframes pulse': {
    '0%, 100%': {
      boxShadow: '0 3px 5px 2px rgba(33, 203, 243, .3)',
    },
    '50%': {
      boxShadow: '0 3px 15px 5px rgba(33, 203, 243, .5)',
    },
  },
}))

/**
 * AskSippyButton - A reusable button that pre-sends a question to the chat widget in
 * a new session.
 *
 * Example usage:
 * ```jsx
 * <AskSippyButton
 *   question="Why is this test failing?"
 *   tooltip="Ask Sippy about this test"
 * />
 * ```
 */
export default function AskSippyButton({ question, tooltip }) {
  const { openDrawer } = useDrawer()
  const { startNewSession } = useSessionActions()
  const { sendMessage } = useWebSocketActions()
  const capabilities = useContext(CapabilitiesContext)
  const classes = useStyles()

  if (!capabilities.includes('chat')) {
    return null
  }

  const handleClick = () => {
    startNewSession()
    openDrawer()
    setTimeout(() => {
      sendMessage(question)
    }, 100)
  }

  const button = (
    <Button
      variant="contained"
      size="medium"
      startIcon={<AutoAwesomeIcon />}
      onClick={handleClick}
      className={classes.defaultStyledButton}
    >
      Ask Sippy
    </Button>
  )

  if (tooltip) {
    return <Tooltip title={tooltip}>{button}</Tooltip>
  }

  return button
}

AskSippyButton.propTypes = {
  question: PropTypes.string.isRequired,
  tooltip: PropTypes.string,
}
