import { BrowserRouter } from 'react-router-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { QueryParamProvider } from 'use-query-params'
import { ReactRouter6Adapter } from 'use-query-params/adapters/react-router-6'
import { render, screen, within } from '@testing-library/react'
import EventEmitter from 'eventemitter3'
import React from 'react'
import TriagedRegressions from './TriagedRegressions'

const theme = createTheme()

const renderWithProviders = (ui) =>
  render(
    <BrowserRouter>
      <QueryParamProvider adapter={ReactRouter6Adapter}>
        <ThemeProvider theme={theme}>{ui}</ThemeProvider>
      </QueryParamProvider>
    </BrowserRouter>
  )

const triageEntries = [
  {
    id: 589,
    description: 'installer failed',
    type: 'product',
    url: 'https://issues.redhat.com/browse/OCPBUGS-1',
    created_at: '2024-09-05T00:00:00Z',
    updated_at: '2024-09-05T00:00:00Z',
    resolved: { Valid: false },
    regressions: [],
  },
  {
    id: 12,
    description: 'installer failed',
    type: 'product',
    url: 'https://issues.redhat.com/browse/OCPBUGS-2',
    created_at: '2024-09-04T00:00:00Z',
    updated_at: '2024-09-04T00:00:00Z',
    resolved: { Valid: false },
    regressions: [],
  },
]

describe('TriagedRegressions ID column', () => {
  test('renders an ID column header and the triage IDs', () => {
    renderWithProviders(
      <TriagedRegressions
        eventEmitter={new EventEmitter()}
        triageEntries={triageEntries}
        allRegressedTests={[]}
      />
    )

    // The new ID column header is present.
    expect(screen.getByRole('columnheader', { name: 'ID' })).toBeInTheDocument()

    // Both triage IDs are shown as visible cell values, which is essential for
    // distinguishing entries with identical descriptions (e.g. installer failures).
    const grid = screen.getByRole('grid')
    expect(within(grid).getByText('589')).toBeInTheDocument()
    expect(within(grid).getByText('12')).toBeInTheDocument()
  })
})
