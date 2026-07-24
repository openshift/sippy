import './index.css'
import { createRoot } from 'react-dom/client'
import { QueryParamProvider } from 'use-query-params'
import { BrowserRouter as Router } from 'react-router-dom'
import App from './App'
import React from 'react'

const root = createRoot(document.getElementById('root'))
root.render(
  <React.StrictMode>
    <Router basename="/sippy-ng">
      <QueryParamProvider options={{ enableBatching: true }}>
        <App />
      </QueryParamProvider>
    </Router>
  </React.StrictMode>
)
