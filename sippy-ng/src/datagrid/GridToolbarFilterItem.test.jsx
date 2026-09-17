import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { render, screen } from '@testing-library/react'
import GridToolbarFilterItem from './GridToolbarFilterItem'
import React from 'react'
import userEvent from '@testing-library/user-event'

vi.mock('./GridToolbarAutocomplete', () => ({ default: () => null }))
vi.mock('./GridToolbarClientAutocomplete', () => ({ default: () => null }))

const theme = createTheme()

const lifecycleColumn = {
  field: 'lifecycle',
  headerName: 'Lifecycle',
  values: ['blocking', 'informing'],
}

function renderItem(props = {}) {
  const defaults = {
    id: 0,
    columns: [lifecycleColumn],
    filterModel: {
      columnField: 'lifecycle',
      operatorValue: 'equals',
      value: '',
      not: false,
    },
    setFilterModel: vi.fn(),
    destroy: vi.fn(),
  }
  return render(
    <ThemeProvider theme={theme}>
      <GridToolbarFilterItem {...defaults} {...props} />
    </ThemeProvider>
  )
}

describe('GridToolbarFilterItem values-restricted column', () => {
  it('restricts the operator dropdown to equals/not-equals', async () => {
    renderItem()
    await userEvent.click(screen.getByLabelText('Operator'))
    const options = screen.getAllByRole('option')
    expect(options.map((o) => o.textContent)).toEqual(['equals', '!='])
  })

  it('renders a fixed dropdown of the configured values', async () => {
    renderItem()
    await userEvent.click(screen.getByLabelText('Value'))
    const options = screen.getAllByRole('option')
    expect(options.map((o) => o.textContent)).toEqual(['blocking', 'informing'])
  })

  it('propagates the Not checkbox alongside the != operator', async () => {
    const setFilterModel = vi.fn()
    renderItem({
      filterModel: {
        columnField: 'lifecycle',
        operatorValue: '!=',
        value: 'blocking',
        not: false,
      },
      setFilterModel,
    })
    await userEvent.click(screen.getByTestId('not-0'))
    expect(setFilterModel).toHaveBeenCalledWith({
      columnField: 'lifecycle',
      not: true,
      operatorValue: '!=',
      value: 'blocking',
    })
  })
})
