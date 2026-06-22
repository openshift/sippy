import { applyFilterModel, evaluateFilter } from './filterUtils'

describe('evaluateFilter', () => {
  const row = { id: 589, description: 'installer failed' }

  test('equals matches exact value', () => {
    const filter = {
      columnField: 'description',
      operatorValue: 'equals',
      value: 'installer failed',
    }
    expect(evaluateFilter(row, filter)).toBe(true)
  })

  test('contains matches substring', () => {
    const filter = {
      columnField: 'description',
      operatorValue: 'contains',
      value: 'installer',
    }
    expect(evaluateFilter(row, filter)).toBe(true)
  })

  // The numeric column type (used by the ID columns) exposes the '=' operator
  // in the filter UI. It must behave like an exact match, not a substring match.
  test("'=' is an exact match for numeric IDs", () => {
    const filter = { columnField: 'id', operatorValue: '=', value: '589' }
    expect(evaluateFilter(row, filter)).toBe(true)
  })

  test("'=' does not match a partial numeric ID", () => {
    const filter = { columnField: 'id', operatorValue: '=', value: '58' }
    expect(evaluateFilter(row, filter)).toBe(false)

    const supersetFilter = {
      columnField: 'id',
      operatorValue: '=',
      value: '5890',
    }
    expect(evaluateFilter(row, supersetFilter)).toBe(false)
  })

  test("'!=' negates an exact numeric match", () => {
    expect(
      evaluateFilter(row, {
        columnField: 'id',
        operatorValue: '!=',
        value: '589',
      })
    ).toBe(false)
    expect(
      evaluateFilter(row, {
        columnField: 'id',
        operatorValue: '!=',
        value: '1',
      })
    ).toBe(true)
  })

  test('numeric comparison operators work on IDs', () => {
    expect(
      evaluateFilter(row, {
        columnField: 'id',
        operatorValue: '>',
        value: '500',
      })
    ).toBe(true)
    expect(
      evaluateFilter(row, {
        columnField: 'id',
        operatorValue: '<',
        value: '500',
      })
    ).toBe(false)
  })

  test('uses column valueGetter when provided', () => {
    const columns = [
      {
        field: 'regression_id',
        valueGetter: (params) => params.row.regression?.id,
      },
    ]
    const regressionRow = { regression: { id: 589 } }
    const filter = {
      columnField: 'regression_id',
      operatorValue: '=',
      value: '589',
    }
    expect(evaluateFilter(regressionRow, filter, columns)).toBe(true)
  })
})

describe('applyFilterModel', () => {
  const rows = [
    { id: 1, description: 'a' },
    { id: 22, description: 'b' },
    { id: 222, description: 'c' },
  ]

  test('filters numeric IDs by exact match', () => {
    const filterModel = {
      items: [{ columnField: 'id', operatorValue: '=', value: '22' }],
    }
    const result = applyFilterModel(rows, filterModel)
    expect(result).toEqual([{ id: 22, description: 'b' }])
  })

  test('returns all rows when there are no valid filters', () => {
    expect(applyFilterModel(rows, { items: [] })).toEqual(rows)
    expect(applyFilterModel(rows, null)).toEqual(rows)
  })
})
