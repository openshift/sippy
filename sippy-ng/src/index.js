import './index.css'
import { createTheme } from '@mui/material/styles'
import { cyan, green, orange, red } from '@mui/material/colors'
import { QueryParamProvider } from 'use-query-params'
import { BrowserRouter as Router } from 'react-router-dom'
import { StyledEngineProvider, ThemeProvider } from '@mui/material'
import App from './App'
import React from 'react'
import ReactDOM from 'react-dom'

ReactDOM.render(
  <React.StrictMode>
    <Router basename="/sippy-ng/">
      <QueryParamProvider options={{ enableBatching: true }}>
        <App />
      </QueryParamProvider>
    </Router>
  </React.StrictMode>,
  document.getElementById('root')
)
