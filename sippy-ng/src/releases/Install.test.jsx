import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import Install from './Install'
import React from 'react'

vi.mock('./InstallTopLevelIndicators', () => ({
  default: ({ links }) => (
    <div data-testid="top-level-indicators-links">
      {links ? 'links-present' : 'links-absent'}
    </div>
  ),
}))

const theme = createTheme()

function jsonResponse(body) {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  }
}

beforeEach(() => {
  vi.restoreAllMocks()
  import.meta.env.VITE_API_URL = ''
})

describe('Install page', () => {
  it('passes the health response links through to InstallTopLevelIndicators', async () => {
    const links = { install: '/api/install?release=quay-3.18' }
    global.fetch = vi.fn((url) => {
      if (url.includes('/api/install'))
        return Promise.resolve(jsonResponse({ column_names: [], tests: {} }))
      if (url.includes('/api/health'))
        return Promise.resolve(jsonResponse({ indicators: {}, links }))
      throw new Error('unexpected fetch url: ' + url)
    })

    render(
      <ThemeProvider theme={theme}>
        <MemoryRouter initialEntries={['/install/quay-3.18/operators']}>
          <Routes>
            <Route
              path="/install/:release/*"
              element={<Install release="quay-3.18" />}
            />
          </Routes>
        </MemoryRouter>
      </ThemeProvider>
    )

    expect(
      await screen.findByTestId('top-level-indicators-links')
    ).toHaveTextContent('links-present')
  })
})
