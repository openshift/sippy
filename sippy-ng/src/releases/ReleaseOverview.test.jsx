import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import React from 'react'
import ReleaseOverview from './ReleaseOverview'

vi.mock('./TopLevelIndicators', () => ({
  default: ({ links }) => (
    <div data-testid="top-level-indicators-links">
      {links ? 'links-present' : 'links-absent'}
    </div>
  ),
}))

vi.mock('./RecentTestFailures', () => ({
  default: () => <div data-testid="recent-test-failures" />,
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

describe('ReleaseOverview page', () => {
  it('passes the health response links through to TopLevelIndicators', async () => {
    const links = { install: '/api/install?release=quay-3.18' }
    global.fetch = vi.fn(() =>
      Promise.resolve(jsonResponse({ indicators: {}, links }))
    )

    render(
      <ThemeProvider theme={theme}>
        <MemoryRouter>
          <ReleaseOverview release="quay-3.18" />
        </MemoryRouter>
      </ThemeProvider>
    )

    expect(
      await screen.findByTestId('top-level-indicators-links')
    ).toHaveTextContent('links-present')
  })
})
