import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import Install from './Install'
import InstallTopLevelIndicators from './InstallTopLevelIndicators'
import React from 'react'
import TopLevelIndicators from './TopLevelIndicators'
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

// Mirrors the real 4.21 API response shape for the OpenShift health card grid.
const health = {
  indicators: {
    infrastructure: indicator({
      name: 'install should succeed: infrastructure',
      current_working_percentage: 99,
      current_runs: 2251,
      previous_working_percentage: 91,
      previous_runs: 2100,
    }),
    installConfig: indicator({
      name: 'install should succeed: configuration',
      current_working_percentage: 98,
      current_runs: 1171,
      previous_working_percentage: 92,
      previous_runs: 1100,
    }),
    bootstrap: indicator({
      name: 'install should succeed: cluster bootstrap',
      current_working_percentage: 97,
      current_runs: 1228,
      previous_working_percentage: 93,
      previous_runs: 1150,
    }),
    installOther: indicator({
      name: 'install should succeed: other',
      current_working_percentage: 96,
      current_runs: 1285,
      previous_working_percentage: 94,
      previous_runs: 1200,
    }),
    install: indicator({
      name: 'install should succeed: overall',
      current_working_percentage: 95,
      current_runs: 1057,
      previous_working_percentage: 85,
      previous_runs: 1000,
    }),
    upgrade: indicator({
      name: '[sig-sippy] upgrade should work',
      current_working_percentage: 94,
      current_runs: 2821,
      previous_working_percentage: 84,
      previous_runs: 2700,
    }),
    tests: indicator({
      name: '[sig-sippy] openshift-tests should work',
      current_working_percentage: 93,
      current_runs: 3049,
      previous_working_percentage: 83,
      previous_runs: 2900,
    }),
  },
}

function variantResult(overrides = {}) {
  return {
    current_pass_percentage: 95,
    previous_pass_percentage: 90,
    current_runs: 100,
    ...overrides,
  }
}

// Mirrors the real 4.21 per-variant operator table response shape.
const install = {
  column_names: ['All', 'aws', 'gcp'],
  tests: {
    'install should succeed: overall': {
      All: variantResult({ current_runs: 1057 }),
      aws: variantResult({ current_pass_percentage: 99, current_runs: 420 }),
      gcp: variantResult({ current_pass_percentage: 93, current_runs: 310 }),
    },
    'operator install authentication': {
      All: variantResult({ current_pass_percentage: 97, current_runs: 900 }),
      aws: variantResult({ current_pass_percentage: 96, current_runs: 360 }),
      gcp: variantResult({ current_pass_percentage: 90, current_runs: 260 }),
    },
  },
}

const upgrade = {
  column_names: ['All', 'aws', 'gcp'],
  tests: {
    '[sig-sippy] upgrade should work': {
      All: variantResult({ current_runs: 2821 }),
      aws: variantResult({ current_pass_percentage: 99, current_runs: 1456 }),
      gcp: variantResult({ current_pass_percentage: 91, current_runs: 700 }),
    },
    'operator upgrade authentication': {
      All: variantResult({ current_pass_percentage: 96, current_runs: 800 }),
      aws: variantResult({ current_pass_percentage: 95, current_runs: 400 }),
      gcp: variantResult({ current_pass_percentage: 89, current_runs: 250 }),
    },
  },
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
