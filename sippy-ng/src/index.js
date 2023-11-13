import './index.css'
import { adaptV4Theme, createTheme } from '@mui/material/styles'
import { QueryParamProvider } from 'use-query-params'
import { BrowserRouter as Router } from 'react-router-dom'
import { ThemeProvider } from '@mui/material'
import App from './App'
import React from 'react'
import ReactDOM from 'react-dom'

// Default theme:
const mode = {
  palette: {
    mode: 'light',
  },
}

ReactDOM.render(
  <React.StrictMode>
    <Router basename="/sippy-ng/">
      <QueryParamProvider options={{ enableBatching: true }}>
        <ThemeProvider theme={createTheme(adaptV4Theme(mode))}>
          <App />
        </ThemeProvider>
      </QueryParamProvider>
    </Router>
  </React.StrictMode>,
  document.getElementById('root')
)
