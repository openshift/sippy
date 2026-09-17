import '@testing-library/jest-dom'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { render, screen } from '@testing-library/react'
import GridToolbar from './GridToolbar'
import React from 'react'
import userEvent from '@testing-library/user-event'

vi.mock('@mui/x-data-grid', () => ({
  GridToolbarDensitySelector: () => null,
}))

vi.mock('./GridToolbarFilterMenu', () => ({
  default: () => null,
}))

const theme = createTheme()

function renderToolbar(props = {}) {
  const defaults = {
    doSearch: vi.fn(),
    clearSearch: vi.fn(),
    setFilterModel: vi.fn(),
    addFilters: vi.fn(),
    filterModel: { items: [] },
  }
  return render(
    <ThemeProvider theme={theme}>
      <GridToolbar {...defaults} {...props} />
    </ThemeProvider>
  )
}

describe('GridToolbar', () => {
  describe('search bar initialization from filter', () => {
    it('populates search from a single positive contains filter', () => {
      renderToolbar({
        searchField: 'name',
        filterModel: {
          items: [
            {
              columnField: 'name',
              operatorValue: 'contains',
              value: 'my-job',
            },
          ],
        },
      })
      expect(screen.getByPlaceholderText('Search…')).toHaveValue('my-job')
    })

    it('does not populate search from a negated contains filter', () => {
      renderToolbar({
        searchField: 'name',
        filterModel: {
          items: [
            {
              columnField: 'name',
              operatorValue: 'contains',
              not: true,
              value: 'excluded',
            },
          ],
        },
      })
      expect(screen.getByPlaceholderText('Search…')).toHaveValue('')
    })

    it('does not populate search when multiple filters exist for the field', () => {
      renderToolbar({
        searchField: 'name',
        filterModel: {
          items: [
            {
              columnField: 'name',
              operatorValue: 'contains',
              value: 'a',
            },
            {
              columnField: 'name',
              operatorValue: 'contains',
              value: 'b',
            },
          ],
        },
      })
      expect(screen.getByPlaceholderText('Search…')).toHaveValue('')
    })

    it('does not populate search when operator is not contains', () => {
      renderToolbar({
        searchField: 'name',
        filterModel: {
          items: [
            {
              columnField: 'name',
              operatorValue: 'equals',
              value: 'exact',
            },
          ],
        },
      })
      expect(screen.getByPlaceholderText('Search…')).toHaveValue('')
    })
  })

  describe('search interactions', () => {
    it('triggers search on Enter key', async () => {
      const doSearch = vi.fn()
      renderToolbar({ doSearch })

      const input = screen.getByPlaceholderText('Search…')
      await userEvent.type(input, 'test-query{enter}')
      expect(doSearch).toHaveBeenCalledWith('test-query')
    })

    it('triggers search on search button click', async () => {
      const doSearch = vi.fn()
      renderToolbar({ doSearch })

      const input = screen.getByPlaceholderText('Search…')
      await userEvent.type(input, 'btn-query')
      await userEvent.click(screen.getByTitle('Search'))
      expect(doSearch).toHaveBeenCalledWith('btn-query')
    })

    it('does not trigger search on blur', async () => {
      const doSearch = vi.fn()
      renderToolbar({ doSearch })

      const input = screen.getByPlaceholderText('Search…')
      await userEvent.type(input, 'blur-query')
      await userEvent.tab()
      expect(doSearch).not.toHaveBeenCalled()
    })

    it('clears search on clear button click', async () => {
      const clearSearch = vi.fn()
      renderToolbar({ clearSearch })

      const input = screen.getByPlaceholderText('Search…')
      await userEvent.type(input, 'something')
      await userEvent.click(screen.getByTitle('Clear'))
      expect(clearSearch).toHaveBeenCalled()
      expect(input).toHaveValue('')
    })
  })
})
