import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import health from './testdata/lifecycle_reports/4.21/health.json'
import install from './testdata/lifecycle_reports/4.21/install.json'
import Install from './Install'
import InstallTopLevelIndicators from './InstallTopLevelIndicators'
import React from 'react'
import TopLevelIndicators from './TopLevelIndicators'
import upgrade from './testdata/lifecycle_reports/4.21/upgrade.json'
import Upgrades from './Upgrades'

const theme = createTheme()

function indicator(overrides = {}) {
  return {
    name: '[sig-quay] install should succeed',
    current_working_percentage: 90,
    current_pass_percentage: 85,
    current_flake_percentage: 5,
    current_failure_percentage: 10,
    current_runs: 20,
    previous_working_percentage: 80,
    previous_runs: 15,
    net_working_improvement: 10,
    ...overrides,
  }
}

function renderWithProviders(children, initialEntries = ['/']) {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>
    </ThemeProvider>
  )
}

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

describe('OpenShift rendering matches base', () => {
  it('TopLevelIndicators for 4.21', () => {
    const { container } = renderWithProviders(
      <TopLevelIndicators release="4.21" indicators={health.indicators} />
    )
    expect(container).toMatchSnapshot()
  })

  it('InstallTopLevelIndicators for 4.21', () => {
    const { container } = renderWithProviders(
      <InstallTopLevelIndicators
        release="4.21"
        indicators={health.indicators}
      />
    )
    expect(container).toMatchSnapshot()
  })

  it('Install page for 4.21', async () => {
    global.fetch = vi.fn((url) => {
      if (url.includes('/api/install'))
        return Promise.resolve(jsonResponse(install))
      if (url.includes('/api/health'))
        return Promise.resolve(jsonResponse(health))
      throw new Error('unexpected fetch url: ' + url)
    })

    const { container } = renderWithProviders(<Install release="4.21" />, [
      '/install/4.21',
    ])
    await screen.findByText('Install health for 4.21')
    expect(container).toMatchSnapshot()
  })

  it('Upgrades page for 4.21', async () => {
    global.fetch = vi.fn((url) => {
      if (url.includes('/api/upgrade'))
        return Promise.resolve(jsonResponse(upgrade))
      throw new Error('unexpected fetch url: ' + url)
    })

    const { container } = renderWithProviders(<Upgrades release="4.21" />, [
      '/upgrade/4.21',
    ])
    await screen.findByText('Upgrade health for 4.21')
    expect(container).toMatchSnapshot()
  })

  it('TopLevelIndicators for 4.10 with legacy infrastructure test name', () => {
    const { container } = renderWithProviders(
      <TopLevelIndicators
        release="4.10"
        indicators={{
          infrastructure: indicator({
            name: '[sig-sippy] infrastructure should work',
          }),
          install: indicator(),
          upgrade: indicator(),
          tests: indicator(),
        }}
      />
    )
    expect(container).toMatchSnapshot()
  })

  it('TopLevelIndicators for 4.21-okd with legacy infrastructure test name', () => {
    const { container } = renderWithProviders(
      <TopLevelIndicators
        release="4.21-okd"
        indicators={{
          infrastructure: indicator({
            name: '[sig-sippy] infrastructure should work',
          }),
          install: indicator(),
          upgrade: indicator(),
          tests: indicator(),
        }}
      />
    )
    expect(container).toMatchSnapshot()
  })
})
