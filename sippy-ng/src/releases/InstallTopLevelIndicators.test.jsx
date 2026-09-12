import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import InstallTopLevelIndicators from './InstallTopLevelIndicators'
import React from 'react'

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
        <InstallTopLevelIndicators
          release={release}
          indicators={indicators}
          links={links}
        />
      </MemoryRouter>
    </ThemeProvider>
  )
}

describe('InstallTopLevelIndicators', () => {
  it('legacy ocp release without new install tests renders nothing', () => {
    const { container } = renderIndicators('4.10', {
      install: indicator(),
    })

    expect(container).toBeEmptyDOMElement()
  })

  it('quay release renders Quay Install and returned OCP phase cards despite the version gate', () => {
    renderIndicators(
      'quay-3.18',
      {
        productInstall: indicator({
          name: '[sig-quay] install should succeed',
        }),
        bootstrap: indicator({ name: '[sig-quay] bootstrap should succeed' }),
        installConfig: indicator({ name: '[sig-quay] config should succeed' }),
      },
      productLinks
    )

    expect(screen.getByText('Quay Install')).toBeInTheDocument()
    expect(screen.getByText('Bootstrap')).toBeInTheDocument()
    expect(screen.getByText('Install-Config')).toBeInTheDocument()
  })

  it('quay release without phase data renders only Quay Install', () => {
    renderIndicators(
      'quay-3.18',
      {
        productInstall: indicator({
          name: '[sig-quay] install should succeed',
        }),
      },
      productLinks
    )

    expect(screen.getByText('Quay Install')).toBeInTheDocument()
    expect(screen.queryByText('Bootstrap')).not.toBeInTheDocument()
    expect(screen.queryByText('Install-Config')).not.toBeInTheDocument()
    expect(screen.queryByText('Infrastructure')).not.toBeInTheDocument()
    expect(screen.queryByText('Install Other')).not.toBeInTheDocument()
  })

  it('quay release with OpenShift indicators but no productInstall data yet still renders the OpenShift cards', () => {
    renderIndicators(
      'quay-3.18',
      {
        install: indicator(),
        infrastructure: indicator({
          name: 'install should succeed: infrastructure',
        }),
      },
      productLinks
    )

    expect(screen.getByText('OpenShift Install')).toBeInTheDocument()
    expect(screen.getByText('Infrastructure')).toBeInTheDocument()
    expect(screen.queryByText('Quay Install')).not.toBeInTheDocument()
  })
})
