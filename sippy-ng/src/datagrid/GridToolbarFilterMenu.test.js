import { fireEvent, render } from '@testing-library/react'
import GridToolbarFilterMenu from './GridToolbarFilterMenu'
import React from 'react'

const columns = [
  {
    field: 'first_name',
    type: 'string',
  },
  {
    field: 'last_name',
  },
  {
    field: 'age',
    type: 'number',
  },
]

describe('GridToolbarFilterMenu', () => {
  it('creates filter model', (done) => {
    let filterModel = {
      items: [],
    }

    const setFilterModel = (m) => {
      filterModel = m
      done()
    }

    const { getByText, getByTestId } = render(
      <GridToolbarFilterMenu
        filterModel={filterModel}
        setFilterModel={setFilterModel}
        columns={columns}
      />
    )

    const filtersButton = getByText('Filters')
    fireEvent.click(filtersButton)

    let columnField = getByTestId('columnField-0')
    let operatorValue = getByTestId('operatorValue-0')
    let value = getByTestId('value-0')

    fireEvent.change(columnField, { target: { value: 'first_name' } })
    fireEvent.change(operatorValue, { target: { value: 'equals' } })
    fireEvent.change(value, { target: { value: 'Ada' } })

    const addButton = getByTestId('add-button')

    fireEvent.click(addButton)
    columnField = getByTestId('columnField-1')
    operatorValue = getByTestId('operatorValue-1')
    value = getByTestId('value-1')

    fireEvent.change(columnField, { target: { value: 'last_name' } })
    fireEvent.change(operatorValue, { target: { value: 'equals' } })
    fireEvent.change(value, { target: { value: 'Lovelace' } })

    const filterButton = getByText('Filter')
    fireEvent.click(filterButton)

    expect(filterModel).toEqual({
      items: [
        {
          columnField: 'first_name',
          not: undefined,
          operatorValue: 'equals',
          value: 'Ada',
        },
        {
          columnField: 'last_name',
          not: undefined,
          operatorValue: 'equals',
          value: 'Lovelace',
        },
      ],
      linkOperator: 'and',
    })
  })
})
