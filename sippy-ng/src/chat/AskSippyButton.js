import { AutoAwesome as AutoAwesomeIcon } from '@mui/icons-material'
import { Button, Tooltip } from '@mui/material'
import { CapabilitiesContext } from '../App'
import { useGlobalChat } from './useGlobalChat'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'

/**
 * AskSippyButton - A reusable button that pre-sends a question to the chat widget
 *
 * Example usage:
 * ```jsx
 * <AskSippyButton
 *   question="Why is this test failing?"
 *   tooltip="Ask Sippy about this test"
 * />
 * ```
 */
export default function AskSippyButton({
  question,
  context,
  tooltip,
  variant = 'outlined',
  size = 'small',
  color = 'primary',
  label = 'Ask Sippy',
  startIcon = <AutoAwesomeIcon />,
  disabled = false,
  sx,
}) {
  const { askQuestion } = useGlobalChat()
  const capabilities = useContext(CapabilitiesContext)

  // Don't render if chat capability is not enabled
  if (!capabilities.includes('chat')) {
    return null
  }

  const handleClick = () => {
    askQuestion(question, context)
  }

  const button = (
    <Button
      variant={variant}
      size={size}
      color={color}
      startIcon={startIcon}
      onClick={handleClick}
      disabled={disabled}
      sx={sx}
    >
      {label}
    </Button>
  )

  if (tooltip) {
    return <Tooltip title={tooltip}>{button}</Tooltip>
  }

  return button
}

AskSippyButton.propTypes = {
  // The question to pre-send to Sippy
  question: PropTypes.string.isRequired,
  // Optional page context to provide to the chat
  context: PropTypes.shape({
    page: PropTypes.string,
    url: PropTypes.string,
    data: PropTypes.object,
    instructions: PropTypes.string,
    suggestedQuestions: PropTypes.arrayOf(PropTypes.string),
  }),
  // Tooltip text (optional)
  tooltip: PropTypes.string,
  // Button variant
  variant: PropTypes.oneOf(['text', 'outlined', 'contained']),
  // Button size
  size: PropTypes.oneOf(['small', 'medium', 'large']),
  // Button color
  color: PropTypes.oneOf([
    'primary',
    'secondary',
    'success',
    'error',
    'info',
    'warning',
  ]),
  // Button label text
  label: PropTypes.string,
  // Start icon (default is AutoAwesomeIcon)
  startIcon: PropTypes.node,
  // Disabled state
  disabled: PropTypes.bool,
  // Additional Material-UI sx prop for custom styling
  sx: PropTypes.object,
}
