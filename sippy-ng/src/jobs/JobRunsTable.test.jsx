import '@testing-library/jest-dom'

/**
 * The requestSearch pattern used in JobRunsTable (and all other table
 * components) immutably replaces the search-field filter while preserving
 * other filters.  We test the pure logic here because the full component
 * requires extensive infrastructure (react-router, query-params, DataGrid,
 * API).
 */
function requestSearch(filterModel, searchField, searchValue) {
  const newItems = filterModel.items.filter(
    (f) => f.columnField !== searchField
  )
  newItems.push({
    id: 99,
    columnField: searchField,
    operatorValue: 'contains',
    value: searchValue,
  })
  return {
    ...filterModel,
    items: newItems,
  }
}

describe('JobRunsTable requestSearch logic', () => {
  it('replaces an existing job filter immutably', () => {
    const original = {
      items: [
        { id: 1, columnField: 'job', operatorValue: 'contains', value: 'old' },
      ],
    }
    const result = requestSearch(original, 'job', 'new-query')
    expect(result.items).toHaveLength(1)
    expect(result.items[0]).toEqual({
      id: 99,
      columnField: 'job',
      operatorValue: 'contains',
      value: 'new-query',
    })
    expect(original.items).toHaveLength(1)
    expect(original.items[0].value).toBe('old')
  })

  it('preserves filters for other fields', () => {
    const original = {
      items: [
        {
          id: 1,
          columnField: 'cluster',
          operatorValue: 'contains',
          value: 'build01',
        },
        {
          id: 2,
          columnField: 'job',
          operatorValue: 'contains',
          value: 'old',
        },
      ],
    }
    const result = requestSearch(original, 'job', 'new-query')
    expect(result.items).toHaveLength(2)
    expect(result.items[0].columnField).toBe('cluster')
    expect(result.items[0].value).toBe('build01')
    expect(result.items[1].value).toBe('new-query')
  })

  it('adds a filter when none exists for the field', () => {
    const original = { items: [] }
    const result = requestSearch(original, 'job', 'my-search')
    expect(result.items).toHaveLength(1)
    expect(result.items[0].value).toBe('my-search')
    expect(original.items).toHaveLength(0)
  })

  it('clears the search by setting an empty value', () => {
    const original = {
      items: [
        {
          id: 99,
          columnField: 'job',
          operatorValue: 'contains',
          value: 'prev',
        },
      ],
    }
    const result = requestSearch(original, 'job', '')
    expect(result.items).toHaveLength(1)
    expect(result.items[0].value).toBe('')
  })
})
