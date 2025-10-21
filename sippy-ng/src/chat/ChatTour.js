import { CONNECTION_STATES } from './store/webSocketSlice'
import {
  useConnectionState,
  useDrawer,
  useSettings,
} from './store/useChatStore'
import { useTheme } from '@mui/material/styles'
import Joyride, { ACTIONS, EVENTS, STATUS } from 'react-joyride'
import PropTypes from 'prop-types'
import React, { useEffect, useState } from 'react'

/**
 * ChatTour - Interactive tour of the chat interface
 * Only shown once per user (tracked via zustand store)
 */
export default function ChatTour({ mode = 'fullPage' }) {
  const theme = useTheme()
  const [runTour, setRunTour] = useState(false)
  const [componentKey, setComponentKey] = useState(0)

  const { settings, setTourCompleted } = useSettings()
  const { connectionState } = useConnectionState()
  const { isDrawerOpen } = useDrawer()

  const isConnected = connectionState === CONNECTION_STATES.CONNECTED
  const tourCompleted = settings.tourCompleted

  // Start tour when ready (drawer mode: only when open)
  useEffect(() => {
    const shouldStartTour =
      isConnected &&
      !tourCompleted &&
      (mode === 'fullPage' || (mode === 'drawer' && isDrawerOpen))

    if (shouldStartTour) {
      const timer = setTimeout(() => {
        if (mode === 'drawer') {
          // Force remount to recalculate positions for drawer
          setComponentKey((prev) => prev + 1)
        }
        setRunTour(true)
      }, 200)
      return () => clearTimeout(timer)
    } else if (mode === 'drawer' && !isDrawerOpen) {
      setRunTour(false)
    }
  }, [isConnected, tourCompleted, mode, isDrawerOpen])

  const handleJoyrideCallback = (data) => {
    const { action, status, type } = data

    if (
      [STATUS.FINISHED, STATUS.SKIPPED].includes(status) ||
      (action === ACTIONS.CLOSE && type === EVENTS.STEP_AFTER)
    ) {
      setRunTour(false)
      setTourCompleted(true)
    }
  }

  // Different selectors for full page vs drawer mode
  const sessionDropdownSelector =
    mode === 'drawer'
      ? '[data-tour="session-dropdown-drawer"]'
      : '[data-tour="session-dropdown"]'

  const steps = [
    {
      target: sessionDropdownSelector,
      content:
        'Click here to view and switch between your chat sessions. Up to the last 50 conversations will be stored locally in your browser.',
      disableBeacon: true,
      placement: 'bottom',
    },
    {
      target: '[data-tour="new-chat"]',
      content:
        'Start a new conversation with Sippy. Each conversation maintains its own context.',
      placement: 'bottom',
    },
    {
      target: '[data-tour="share-button"]',
      content:
        'Share your conversation with others. This creates a shareable link that anyone can view.',
      placement: 'bottom',
    },
    {
      target: '[data-tour="help-button"]',
      content:
        'Need help? Click here to open the Sippy Chat user guide with detailed instructions and examples.',
      placement: 'bottom',
    },
    {
      target: '[data-tour="settings-button"]',
      content:
        'Customize Sippy chat settings: thinking steps, local storage, connection options, and more.',
      placement: 'bottom',
    },
    {
      target: '[data-tour="status-area"]',
      content:
        "Status information appears here: connection status, contextual information from the page you're viewing, and your selected AI persona.",
      placement: 'top',
    },
    {
      target: '[data-tour="suggestions"]',
      content:
        'Not sure what to ask? Click any of these suggestions to get started.',
      placement: 'top',
    },
    {
      target: '[data-tour="command-button"]',
      content:
        'Browse all available prompt commands. You can also type "/" in the input to search commands.',
      placement: 'top',
    },
  ]

  return (
    <Joyride
      key={componentKey}
      steps={steps}
      run={runTour}
      continuous
      showProgress
      showSkipButton
      hideCloseButton
      callback={handleJoyrideCallback}
      disableScrolling={mode === 'drawer'}
      scrollToFirstStep={mode === 'fullPage'}
      styles={{
        options: {
          arrowColor: theme.palette.background.paper,
          backgroundColor: theme.palette.background.paper,
          primaryColor: theme.palette.primary.main,
          textColor: theme.palette.text.primary,
          width: 360,
          zIndex: 1250,
        },
        tooltip: {
          borderRadius: theme.shape.borderRadius * 2,
          fontSize: 14,
          padding: '16px',
        },
        tooltipContent: {
          textAlign: 'left',
        },
        buttonNext: {
          backgroundColor: theme.palette.primary.main,
          borderRadius: theme.shape.borderRadius,
          fontSize: 14,
          padding: '8px 16px',
        },
        buttonBack: {
          color: theme.palette.text.primary,
          marginRight: 10,
          fontSize: 14,
          padding: '8px 16px',
          borderRadius: theme.shape.borderRadius,
          border: `1px solid ${theme.palette.divider}`,
        },
        buttonSkip: {
          color: theme.palette.text.secondary,
          fontSize: 14,
          padding: '8px 12px',
          borderRadius: theme.shape.borderRadius,
          border: `1px solid ${theme.palette.divider}`,
          backgroundColor: 'transparent',
        },
        spotlight: {
          borderRadius: theme.shape.borderRadius,
        },
      }}
      locale={{
        back: 'Back',
        close: 'Close',
        last: 'Got it!',
        next: 'Next',
        skip: 'Skip tour',
      }}
      floaterProps={{
        disableAnimation: true,
        ...(mode === 'drawer' && { disableFlip: true }),
      }}
      spotlightClicks
      spotlightPadding={mode === 'drawer' ? 5 : 10}
    />
  )
}

ChatTour.propTypes = {
  mode: PropTypes.oneOf(['fullPage', 'drawer']),
}
