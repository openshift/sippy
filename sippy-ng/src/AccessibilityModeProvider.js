import { AccessibilityModeContext } from './App'
import { useCookies } from 'react-cookie'
import PropTypes from 'prop-types'
import React from 'react'

export const AccessibilityModeProvider = ({ children }) => {
  const [cookies, setCookie] = useCookies(['sippyAccessibilityMode'])

  const accessibilityModePreference = cookies['sippyAccessibilityMode']

  const [accessibilityModeOn, setAccessibilityMode] = React.useState(
    accessibilityModePreference
  )

  const toggleAccessibilityMode = () => {
    setAccessibilityMode((prevMode) => {
      const newMode = !prevMode
      setCookie('sippyAccessibilityMode', newMode, {
        path: '/',
        sameSite: 'Strict',
        expires: new Date('3000-12-31'),
      })
      return newMode
    })
  }

  return (
    <AccessibilityModeContext.Provider
      value={{ accessibilityModeOn, toggleAccessibilityMode }}
    >
      {children}
    </AccessibilityModeContext.Provider>
  )
}

AccessibilityModeProvider.propTypes = {
  children: PropTypes.node.isRequired,
}
