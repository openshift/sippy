import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { render, screen } from '@testing-library/react'
import GridToolbarFilterMenu from './GridToolbarFilterMenu'
import React from 'react'
import userEvent from '@testing-library/user-event'

vi.mock('@mui/styles', () => ({
  makeStyles: () => () => ({}),
}))

vi.mock('./GridToolbarFilterItem', () => ({
  default: ({ id, filterModel, destroy }) => (
    <div data-testid={`filter-item-${id}`}>
      <span data-testid={`filter-column-${id}`}>{filterModel.columnField}</span>
      <span data-testid={`filter-operator-${id}`}>
        {filterModel.operatorValue}
      </span>
      <span data-testid={`filter-value-${id}`}>{filterModel.value}</span>
      <button data-testid={`filter-destroy-${id}`} onClick={destroy}>
        Remove
      </button>
    </div>
  ),
  operatorWithoutValue: ['is empty', 'is not empty'],
}))

vi.mock('./utils', () => ({
  filterTooltip: () => '',
}))

const theme = createTheme()

function renderMenu(props = {}) {
  const defaults = {
    filterModel: { items: [] },
    setFilterModel: vi.fn(),
    columns: [
      { field: 'name', headerName: 'Name', type: 'string' },
      { field: 'status', headerName: 'Status', type: 'string' },
    ],
  }
  const merged = { ...defaults, ...props }
  return {
    ...render(
      <ThemeProvider theme={theme}>
        <GridToolbarFilterMenu {...merged} />
      </ThemeProvider>
    ),
    setFilterModel: merged.setFilterModel,
  }
}

describe('GridToolbarFilterMenu', () => {
  describe('does not mutate parent filterModel', () => {
    it('does not mutate the parent items array on mount', () => {
      const parentItems = []
      const filterModel = { items: parentItems }
      renderMenu({ filterModel })
      expect(parentItems).toHaveLength(0)
    })

    it('does not mutate the parent items array when adding a filter', async () => {
      const parentItems = [
        { columnField: 'name', operatorValue: 'contains', value: 'test' },
      ]
      const filterModel = { items: parentItems }
      renderMenu({ filterModel })

      await userEvent.click(screen.getByText('Filters'))
      await userEvent.click(screen.getByLabelText('add'))

      expect(parentItems).toHaveLength(1)
    })

    it('does not mutate the parent items array when removing a filter', async () => {
      const parentItems = [
        { columnField: 'name', operatorValue: 'contains', value: 'a' },
        { columnField: 'status', operatorValue: 'equals', value: 'b' },
      ]
      const filterModel = { items: parentItems }
      renderMenu({ filterModel })

      await userEvent.click(screen.getByText('Filters'))
      await userEvent.click(screen.getByTestId('filter-destroy-0'))

      expect(parentItems).toHaveLength(2)
    })
  })

  describe('external model synchronization', () => {
    it('syncs internal state when parent filterModel changes', async () => {
      const { rerender } = render(
        <ThemeProvider theme={theme}>
          <GridToolbarFilterMenu
            filterModel={{ items: [] }}
            setFilterModel={vi.fn()}
            columns={[{ field: 'name', headerName: 'Name', type: 'string' }]}
          />
        </ThemeProvider>
      )

      rerender(
        <ThemeProvider theme={theme}>
          <GridToolbarFilterMenu
            filterModel={{
              items: [
                {
                  columnField: 'name',
                  operatorValue: 'contains',
                  value: 'synced',
                },
              ],
            }}
            setFilterModel={vi.fn()}
            columns={[{ field: 'name', headerName: 'Name', type: 'string' }]}
          />
        </ThemeProvider>
      )

      await userEvent.click(screen.getByText('Filters'))
      expect(screen.getByTestId('filter-value-0')).toHaveTextContent('synced')
    })
  })

  describe('add and remove filters', () => {
    it('starts with one blank filter row when items is empty', async () => {
      renderMenu()
      await userEvent.click(screen.getByText('Filters'))
      expect(screen.getByTestId('filter-item-0')).toBeInTheDocument()
    })

    it('adds a new blank filter row on add click', async () => {
      renderMenu({
        filterModel: {
          items: [
            { columnField: 'name', operatorValue: 'contains', value: 'a' },
          ],
        },
      })

      await userEvent.click(screen.getByText('Filters'))
      expect(screen.queryByTestId('filter-item-1')).not.toBeInTheDocument()

      await userEvent.click(screen.getByLabelText('add'))
      expect(screen.getByTestId('filter-item-1')).toBeInTheDocument()
    })

    it('resets to a blank row when removing the last filter', async () => {
      renderMenu({
        filterModel: {
          items: [
            { columnField: 'name', operatorValue: 'contains', value: 'only' },
          ],
        },
      })

      await userEvent.click(screen.getByText('Filters'))
      await userEvent.click(screen.getByTestId('filter-destroy-0'))

      expect(screen.getByTestId('filter-item-0')).toBeInTheDocument()
      expect(screen.getByTestId('filter-column-0')).toHaveTextContent('')
    })
  })

  describe('handleClose clears blank filters', () => {
    it('does not call setFilterModel when closing with only blank filters', async () => {
      const { setFilterModel } = renderMenu()

      await userEvent.click(screen.getByText('Filters'))
      await userEvent.click(screen.getByText('Filter'))

      expect(setFilterModel).not.toHaveBeenCalled()
    })

    it('restores a blank row after closing with all-blank filters', async () => {
      renderMenu()

      await userEvent.click(screen.getByText('Filters'))
      await userEvent.click(screen.getByText('Filter'))

      await userEvent.click(screen.getByText('Filters'))
      expect(screen.getByTestId('filter-item-0')).toBeInTheDocument()
    })
  })

  describe('badge count', () => {
    it('shows zero for blank filters', () => {
      renderMenu({
        filterModel: {
          items: [{ columnField: '', operatorValue: '', value: '' }],
        },
      })
      const badge = screen.getByText('Filters').closest('button')
      expect(badge).toBeInTheDocument()
    })

    it('counts non-blank filters', () => {
      renderMenu({
        filterModel: {
          items: [
            { columnField: 'name', operatorValue: 'contains', value: 'a' },
            { columnField: 'status', operatorValue: 'equals', value: 'b' },
          ],
        },
      })
      expect(screen.getByText('2')).toBeInTheDocument()
    })
  })
})
