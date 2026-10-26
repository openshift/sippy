import '@testing-library/jest-dom'
import { cleanup, render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { MemoryRouter } from 'react-router'
import { pathForExactTestAnalysisWithVariants } from '../helpers'
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

function renderComponent(regressions) {
  mocks.dataGridProps = null
  render(
    <MemoryRouter>
      <ThemeProvider theme={createTheme()}>
        <TriagedRegressionTestList regressions={regressions || [{ id: 1 }]} />
      </ThemeProvider>
    </MemoryRouter>
  )
}

function getColumn(field) {
  return mocks.dataGridProps.columns.find((column) => column.field === field)
}

function getForceClosedColumn() {
  renderComponent()
  return getColumn('force_closed')
}

test('renders an empty grid when regressions is null', () => {
  mocks.dataGridProps = null
  render(
    <ThemeProvider theme={createTheme()}>
      <TriagedRegressionTestList regressions={null} />
    </ThemeProvider>
  )
  expect(mocks.dataGridProps.rows).toEqual([])
})

describe('TriagedRegressionTestList force-closed indicator', () => {
  test('is filterable, provides Yes and No values, and hides the icon when not force closed', () => {
    const column = getForceClosedColumn()

    expect(column.filterable).toBe(true)
    expect(column.valueGetter(undefined, { force_closed: true })).toBe('Yes')
    expect(column.valueGetter(undefined, { force_closed: false })).toBe('No')
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

describe('TriagedRegressionTestList test_name links', () => {
  const testName =
    '[sig-network] pod connected to UserDefinedNetwork [Suite:openshift/conformance/parallel]'
  const variants = [
    'Architecture:amd64',
    'Platform:gcp',
    'Network:ovn',
    'Suite:unknown',
    'Upgrade:none',
    'FeatureSet:default',
    'Installer:ipi',
    'LayeredProduct:none',
    'Topology:ha',
  ]

  test('renders test name as a React Router Link with correct path', () => {
    renderComponent()
    const column = getColumn('test_name')

    const cell = render(
      <MemoryRouter>
        {column.renderCell({
          value: testName,
          row: { test_name: testName, release: '5.0', variants },
        })}
      </MemoryRouter>
    )

    const link = cell.getByRole('link')
    expect(link).toHaveTextContent(testName)
    expect(link).toHaveAttribute(
      'href',
      pathForExactTestAnalysisWithVariants('5.0', testName, variants)
    )
  })

  test('two rows with same test but different variants generate different links', () => {
    renderComponent()
    const column = getColumn('test_name')

    const variantsA = ['Platform:gcp', 'Network:ovn']
    const variantsB = ['Platform:aws', 'Network:sdn']

    const cellA = render(
      <MemoryRouter>
        {column.renderCell({
          value: testName,
          row: { test_name: testName, release: '5.0', variants: variantsA },
        })}
      </MemoryRouter>
    )
    const hrefA = cellA.getByRole('link').getAttribute('href')
    cleanup()

    const cellB = render(
      <MemoryRouter>
        {column.renderCell({
          value: testName,
          row: { test_name: testName, release: '5.0', variants: variantsB },
        })}
      </MemoryRouter>
    )
    const hrefB = cellB.getByRole('link').getAttribute('href')

    expect(hrefA).not.toBe(hrefB)
  })

  test('rows from different releases navigate to their own release', () => {
    renderComponent()
    const column = getColumn('test_name')

    const cell50 = render(
      <MemoryRouter>
        {column.renderCell({
          value: testName,
          row: { test_name: testName, release: '5.0', variants: [] },
        })}
      </MemoryRouter>
    )
    expect(cell50.getByRole('link').getAttribute('href')).toMatch(
      /^\/tests\/5\.0\/analysis/
    )
    cleanup()

    const cell416 = render(
      <MemoryRouter>
        {column.renderCell({
          value: testName,
          row: { test_name: testName, release: '4.16', variants: [] },
        })}
      </MemoryRouter>
    )
    expect(cell416.getByRole('link').getAttribute('href')).toMatch(
      /^\/tests\/4\.16\/analysis/
    )
  })

  test('falls back to analysis across variants when row has no variants', () => {
    renderComponent()
    const column = getColumn('test_name')

    const cell = render(
      <MemoryRouter>
        {column.renderCell({
          value: testName,
          row: { test_name: testName, release: '5.0', variants: [] },
        })}
      </MemoryRouter>
    )

    const href = cell.getByRole('link').getAttribute('href')
    const filtersParam = new URLSearchParams(href.split('?')[1]).get('filters')
    const filters = JSON.parse(filtersParam)

    const positiveVariants = filters.items.filter(
      (f) => f.field === 'variants' && !f.not
    )
    expect(positiveVariants).toHaveLength(0)
  })
})
