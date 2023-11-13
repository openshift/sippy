import './index.css'
import { createTheme } from '@mui/material/styles'
import { green, orange, red } from '@mui/material/colors'
import { QueryParamProvider } from 'use-query-params'
import { BrowserRouter as Router } from 'react-router-dom'
import { ThemeProvider } from '@mui/material'
import App from './App'
import React from 'react'
import ReactDOM from 'react-dom'

// Default theme, restore settings from v4 color schemes. v5 is much darker.
const lightTheme = {
  palette: {
    mode: 'light',
    success: {
      main: green[500],
      light: green[300],
      dark: green[700],
    },
    warning: {
      main: orange[500],
      light: orange[300],
      dark: orange[700],
    },
    error: {
      main: red[500],
      light: red[300],
      dark: red[700],
    },
  },
}

ReactDOM.render(
  <React.StrictMode>
    <Router basename="/sippy-ng/">
      <QueryParamProvider options={{ enableBatching: true }}>
        <ThemeProvider theme={createTheme(lightTheme)}>
          <App />
        </ThemeProvider>
      </QueryParamProvider>
    </Router>
  </React.StrictMode>,
  document.getElementById('root')
)
