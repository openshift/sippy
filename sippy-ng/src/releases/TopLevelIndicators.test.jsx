import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import React from 'react'
import TopLevelIndicators from './TopLevelIndicators'

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

// productLinks mirrors the HATEOAS links /api/health returns for a product release
// (pkg/api/lifecycle_tests.go lifecycleLinks); only their presence matters here.
const productLinks = {
  install: '/api/install?release=quay-3.18',
  upgrade: '/api/upgrade?release=quay-3.18',
  health: '/api/health?release=quay-3.18',
  product_install_test: '/api/tests?release=quay-3.18',
  product_upgrade_test: '/api/tests?release=quay-3.18',
}

function renderIndicators(release, indicators, links) {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <TopLevelIndicators
          release={release}
          indicators={indicators}
          links={links}
        />
      </MemoryRouter>
    </ThemeProvider>
  )
}

describe('TopLevelIndicators', () => {
  it('ocp release renders Install and Upgrade labels and no product cards', () => {
    renderIndicators('4.21', {
      install: indicator(),
      upgrade: indicator(),
    })

    expect(screen.getByText('Install')).toBeInTheDocument()
    expect(screen.getByText('Upgrade')).toBeInTheDocument()
    expect(screen.queryByText('OpenShift Install')).not.toBeInTheDocument()
    expect(screen.queryByText('OpenShift Upgrade')).not.toBeInTheDocument()
    expect(screen.queryByText('Quay Install')).not.toBeInTheDocument()
    expect(screen.queryByText('Quay Upgrade')).not.toBeInTheDocument()
  })

  it('quay release renders OpenShift Install, OpenShift Upgrade, Quay Install and Quay Upgrade', () => {
    renderIndicators(
      'quay-3.18',
      {
        install: indicator(),
        upgrade: indicator(),
        productInstall: indicator({
          name: '[sig-quay] install should succeed',
        }),
        productUpgrade: indicator({
          name: '[sig-quay] upgrade should succeed',
        }),
      },
      productLinks
    )

    expect(screen.getByText('OpenShift Install')).toBeInTheDocument()
    expect(screen.getByText('OpenShift Upgrade')).toBeInTheDocument()
    expect(screen.getByText('Quay Install')).toBeInTheDocument()
    expect(screen.getByText('Quay Upgrade')).toBeInTheDocument()
  })

  it('quay release with missing productUpgrade renders no Quay Upgrade card', () => {
    renderIndicators(
      'quay-3.18',
      {
        install: indicator(),
        upgrade: indicator(),
        productInstall: indicator({
          name: '[sig-quay] install should succeed',
        }),
      },
      productLinks
    )

    expect(screen.getByText('Quay Install')).toBeInTheDocument()
    expect(screen.queryByText('Quay Upgrade')).not.toBeInTheDocument()
  })

  it('quay release Infrastructure card links to the returned indicator name', () => {
    renderIndicators(
      'quay-3.18',
      {
        infrastructure: indicator({
          name: 'install should succeed: infrastructure',
        }),
        productInstall: indicator({
          name: '[sig-quay] install should succeed',
        }),
      },
      productLinks
    )

    const link = screen.getByText('Infrastructure').closest('a')
    expect(link).toHaveAttribute(
      'href',
      expect.stringContaining(
        encodeURIComponent('install should succeed: infrastructure')
      )
    )
  })

  it('quay release Infrastructure card links to the returned indicator name even before product data has reported', () => {
    renderIndicators(
      'quay-3.18',
      {
        infrastructure: indicator({
          name: 'install should succeed: infrastructure',
        }),
      },
      productLinks
    )

    const link = screen.getByText('Infrastructure').closest('a')
    expect(link).toHaveAttribute(
      'href',
      expect.stringContaining(
        encodeURIComponent('install should succeed: infrastructure')
      )
    )
  })
})
