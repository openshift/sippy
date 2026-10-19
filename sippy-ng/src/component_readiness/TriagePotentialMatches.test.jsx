import '@testing-library/jest-dom'
import { CompReadyVarsContext } from './CompReadyVars'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { render, screen } from '@testing-library/react'
import React from 'react'
import TriagePotentialMatches from './TriagePotentialMatches'

vi.mock('use-query-params', () => ({
  BooleanParam: {},
  useQueryParam: () => [undefined, vi.fn()],
}))

vi.mock('./CompSeverityIcon', () => ({ default: () => null }))

function renderComponent(triage) {
  return render(
    <ThemeProvider theme={createTheme()}>
      <CompReadyVarsContext.Provider value={{ view: 'test-view' }}>
        <TriagePotentialMatches
          triage={triage}
          setMessage={vi.fn()}
          setLinkingComplete={vi.fn()}
        />
      </CompReadyVarsContext.Provider>
    </ThemeProvider>
  )
}

describe('TriagePotentialMatches button state', () => {
  test('is disabled when there are no active views (all regressions rolled off)', () => {
    const triage = {
      id: 1,
      regressions: [{ id: 10, views: [{ view_name: 'v1', active: false }] }],
      links: { potential_matches: '/api/triages/1/matches' },
    }

    renderComponent(triage)

    const button = screen.getByRole('button', {
      name: /link additional matching regressions/i,
    })
    expect(button).toBeDisabled()
  })

  test('is disabled when there are no regressions', () => {
    const triage = {
      id: 2,
      regressions: [],
      links: { potential_matches: '/api/triages/2/matches' },
    }

    renderComponent(triage)

    const button = screen.getByRole('button', {
      name: /link additional matching regressions/i,
    })
    expect(button).toBeDisabled()
  })

  test('is enabled when there are active views', () => {
    const triage = {
      id: 3,
      regressions: [{ id: 10, views: [{ view_name: 'v1', active: true }] }],
      links: { potential_matches: '/api/triages/3/matches' },
    }

    renderComponent(triage)

    const button = screen.getByRole('button', {
      name: /link additional matching regressions/i,
    })
    expect(button).toBeEnabled()
  })
})
