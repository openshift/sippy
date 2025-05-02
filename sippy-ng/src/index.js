import './index.css'
import { QueryParamProvider } from 'use-query-params'
import { BrowserRouter as Router } from 'react-router-dom'
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
