import { AutoAwesome as AutoAwesomeIcon } from '@mui/icons-material'
import { Button, Snackbar, Tooltip } from '@mui/material'
import { CapabilitiesContext } from '../App'
import { makeStyles } from '@mui/styles'
import { useDrawer, usePrompts, useSessionActions } from './store/useChatStore'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { useContext, useState } from 'react'

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
 * a new session. Can be used with either a direct question or a slash command.
 *
 * Example usage with direct question:
 * ```jsx
 * <AskSippyButton
 *   question="Why is this test failing?"
 *   tooltip="Ask Sippy about this test"
 * />
 * ```
 *
 * Example usage with slash command:
 * ```jsx
 * <AskSippyButton
 *   slashCommand="test-details-analysis"
 *   commandArgs={{ url: window.location.href }}
 *   tooltip="Analyze this test regression"
 * />
 * ```
 */
export default function AskSippyButton({
  question,
  slashCommand,
  commandArgs,
  tooltip,
}) {
  const { openDrawer } = useDrawer()
  const { startNewSession } = useSessionActions()
  const { renderPrompt } = usePrompts()
  const capabilities = useContext(CapabilitiesContext)
  const classes = useStyles()
  const [isRendering, setIsRendering] = useState(false)
  const [error, setError] = useState(null)

  if (!capabilities.includes('chat')) {
    return null
  }

  const handleClick = async () => {
    // If using a slash command, render the prompt first
    if (slashCommand && commandArgs) {
      setIsRendering(true)
      setError(null)
      try {
        const rendered = await renderPrompt(slashCommand, commandArgs)
        openDrawer()
        startNewSession(rendered)
      } catch (err) {
        console.error('Failed to render prompt:', err)
        setError(
          `Failed to load prompt '${slashCommand}': ${err.message || err}`
        )
      } finally {
        setIsRendering(false)
      }
    } else if (question) {
      openDrawer()
      startNewSession(question)
    }
  }

  const handleCloseError = () => {
    setError(null)
  }

  const button = (
    <Button
      variant="contained"
      size="medium"
      startIcon={<AutoAwesomeIcon />}
      onClick={handleClick}
      className={classes.defaultStyledButton}
      disabled={isRendering}
    >
      {isRendering ? 'Loading...' : 'Ask Sippy'}
    </Button>
  )

  return (
    <>
      {tooltip ? <Tooltip title={tooltip}>{button}</Tooltip> : button}
      <Snackbar
        open={!!error}
        autoHideDuration={6000}
        onClose={handleCloseError}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={handleCloseError}
          severity="error"
          sx={{ width: '100%' }}
        >
          {error}
        </Alert>
      </Snackbar>
    </>
  )
}

AskSippyButton.propTypes = {
  question: PropTypes.string,
  slashCommand: PropTypes.string,
  commandArgs: PropTypes.object,
  tooltip: PropTypes.string,
}
