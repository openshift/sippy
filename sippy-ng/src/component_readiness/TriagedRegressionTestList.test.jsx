import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { render, screen } from '@testing-library/react'
import React from 'react'
import TriagedRegressionTestList from './TriagedRegressionTestList'
import userEvent from '@testing-library/user-event'

const mocks = vi.hoisted(() => ({ dataGridProps: null }))

vi.mock('@mui/x-data-grid', () => ({
  DataGrid: (props) => {
    mocks.dataGridProps = props
    return <div data-testid="data-grid" />
  },
}))

vi.mock('use-query-params', () => ({
  NumberParam: {},
  useQueryParam: (name) =>
    name === 'regressedModalTestFilters'
      ? [{ items: [] }, vi.fn()]
      : [0, vi.fn()],
}))

function getForceClosedColumn() {
  mocks.dataGridProps = null
  render(
    <ThemeProvider theme={createTheme()}>
      <TriagedRegressionTestList regressions={[{ id: 1 }]} />
    </ThemeProvider>
  )
  return mocks.dataGridProps.columns.find(
    (column) => column.field === 'force_closed'
  )
}

describe('TriagedRegressionTestList force-closed indicator', () => {
  test('is filterable, provides Yes and No values, and hides the icon when not force closed', () => {
    const column = getForceClosedColumn()

    expect(column.filterable).toBe(true)
    expect(column.valueGetter({ row: { force_closed: true } })).toBe('Yes')
    expect(column.valueGetter({ row: { force_closed: false } })).toBe('No')
    expect(column.renderCell({ row: { force_closed: false } })).toBeNull()
  })

  test('shows force-close attribution metadata in the tooltip', async () => {
    const column = getForceClosedColumn()
    render(
      column.renderCell({
        row: {
          force_closed: true,
          force_closed_reason: 'stale regression',
          force_closed_by: 'developer',
          force_closed_by_triage_id: 42,
        },
      })
    )

    await userEvent.hover(screen.getByTestId('CheckCircleIcon'))

    const tooltipContent = await screen.findByText(/Reason: stale regression/)
    expect(tooltipContent).toBeInTheDocument()
    expect(tooltipContent).toHaveStyle({ whiteSpace: 'pre-line' })
    expect(tooltipContent).not.toHaveAttribute('style')
    expect(screen.getByText(/By: developer/)).toBeInTheDocument()
    expect(screen.getByText(/Triage: 42/)).toBeInTheDocument()
  })

  test('uses a fallback tooltip when attribution metadata is absent', async () => {
    const column = getForceClosedColumn()
    render(column.renderCell({ row: { force_closed: true } }))

    await userEvent.hover(screen.getByTestId('CheckCircleIcon'))

    expect(await screen.findByText('Force closed')).toBeInTheDocument()
  })
})
