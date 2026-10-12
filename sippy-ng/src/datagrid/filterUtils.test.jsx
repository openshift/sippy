import {
  applyFilterModel,
  evaluateFilter,
  normalizeFilterItem,
  normalizeFilterModel,
  shouldKeepFilterItem,
  VALUELESS_OPERATORS,
} from './filterUtils'
import { describe, expect, it } from 'vitest'

describe('normalizeFilterItem', () => {
  it('maps v5 columnField to field and strips legacy aliases', () => {
    const item = { columnField: 'name', operatorValue: 'contains', value: 'x' }
    const result = normalizeFilterItem(item)
    expect(result.field).toBe('name')
    expect(result.operator).toBe('contains')
    expect(result).not.toHaveProperty('columnField')
    expect(result).not.toHaveProperty('operatorValue')
  })

  it('preserves v7 field/operator when both generations present and strips legacy', () => {
    const item = {
      field: 'v7name',
      columnField: 'v5name',
      operator: 'equals',
      operatorValue: 'contains',
      value: 'x',
    }
    const result = normalizeFilterItem(item)
    expect(result.field).toBe('v7name')
    expect(result.operator).toBe('equals')
    expect(result).not.toHaveProperty('columnField')
    expect(result).not.toHaveProperty('operatorValue')
  })

  it('passes through v7-only items unchanged', () => {
    const item = { field: 'name', operator: 'contains', value: 'x' }
    const result = normalizeFilterItem(item)
    expect(result.field).toBe('name')
    expect(result.operator).toBe('contains')
  })

  it('returns null/undefined as-is', () => {
    expect(normalizeFilterItem(null)).toBeNull()
    expect(normalizeFilterItem(undefined)).toBeUndefined()
  })
})

describe('normalizeFilterModel', () => {
  it('maps v5 linkOperator to logicOperator and strips legacy alias', () => {
    const model = {
      items: [{ columnField: 'name', operatorValue: 'contains', value: 'x' }],
      linkOperator: 'or',
    }
    const result = normalizeFilterModel(model)
    expect(result.logicOperator).toBe('or')
    expect(result).not.toHaveProperty('linkOperator')
    expect(result.items[0].field).toBe('name')
    expect(result.items[0].operator).toBe('contains')
    expect(result.items[0]).not.toHaveProperty('columnField')
    expect(result.items[0]).not.toHaveProperty('operatorValue')
  })

  it('preserves v7 logicOperator when both present and strips legacy', () => {
    const model = {
      items: [],
      logicOperator: 'and',
      linkOperator: 'or',
    }
    const result = normalizeFilterModel(model)
    expect(result.logicOperator).toBe('and')
    expect(result).not.toHaveProperty('linkOperator')
  })

  it('returns null/undefined as-is', () => {
    expect(normalizeFilterModel(null)).toBeNull()
    expect(normalizeFilterModel(undefined)).toBeUndefined()
  })
})

describe('shouldKeepFilterItem', () => {
  it('keeps items with non-empty value', () => {
    expect(shouldKeepFilterItem({ value: 'test', operator: 'contains' })).toBe(
      true
    )
  })

  it('filters out items with empty value and non-valueless operator', () => {
    expect(shouldKeepFilterItem({ value: '', operator: 'contains' })).toBe(
      false
    )
  })

  it.each(VALUELESS_OPERATORS)(
    'keeps items with %s operator even without value',
    (op) => {
      expect(shouldKeepFilterItem({ value: '', operator: op })).toBe(true)
    }
  )

  it('keeps items with v5 operatorValue for valueless operators', () => {
    expect(shouldKeepFilterItem({ value: '', operatorValue: 'isEmpty' })).toBe(
      true
    )
  })
})

describe('evaluateFilter', () => {
  const row = {
    name: 'Hello World',
    count: 42,
    description: '',
    missing: null,
  }

  describe('contains operator', () => {
    it('matches when field contains value (case-insensitive)', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'contains',
          value: 'hello',
        })
      ).toBe(true)
    })

    it('does not match when field does not contain value', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'contains',
          value: 'xyz',
        })
      ).toBe(false)
    })
  })

  describe('equals operator', () => {
    it('matches exact value (case-insensitive)', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'equals',
          value: 'hello world',
        })
      ).toBe(true)
    })

    it('does not match partial value', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'equals',
          value: 'hello',
        })
      ).toBe(false)
    })
  })

  describe('startsWith operator', () => {
    it('matches prefix', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'startsWith',
          value: 'hello',
        })
      ).toBe(true)
    })
  })

  describe('endsWith operator', () => {
    it('matches suffix', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'endsWith',
          value: 'world',
        })
      ).toBe(true)
    })
  })

  describe('comparison operators', () => {
    it('> compares numerically', () => {
      expect(
        evaluateFilter(row, { field: 'count', operator: '>', value: '40' })
      ).toBe(true)
      expect(
        evaluateFilter(row, { field: 'count', operator: '>', value: '50' })
      ).toBe(false)
    })

    it('>= compares numerically', () => {
      expect(
        evaluateFilter(row, { field: 'count', operator: '>=', value: '42' })
      ).toBe(true)
    })

    it('< compares numerically', () => {
      expect(
        evaluateFilter(row, { field: 'count', operator: '<', value: '50' })
      ).toBe(true)
    })

    it('<= compares numerically', () => {
      expect(
        evaluateFilter(row, { field: 'count', operator: '<=', value: '42' })
      ).toBe(true)
    })
  })

  describe('not equals operator', () => {
    it('!= returns true for different values', () => {
      expect(
        evaluateFilter(row, { field: 'name', operator: '!=', value: 'other' })
      ).toBe(true)
    })

    it('not equals returns true for different values', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'not equals',
          value: 'other',
        })
      ).toBe(true)
    })
  })

  describe('isEmpty/isNotEmpty operators', () => {
    it('isEmpty matches null fields', () => {
      expect(
        evaluateFilter(row, { field: 'missing', operator: 'isEmpty' })
      ).toBe(true)
    })

    it('isEmpty matches empty string fields', () => {
      expect(
        evaluateFilter(row, { field: 'description', operator: 'isEmpty' })
      ).toBe(true)
    })

    it('isEmpty does not match non-empty fields', () => {
      expect(evaluateFilter(row, { field: 'name', operator: 'isEmpty' })).toBe(
        false
      )
    })

    it('isNotEmpty matches non-empty fields', () => {
      expect(
        evaluateFilter(row, { field: 'name', operator: 'isNotEmpty' })
      ).toBe(true)
    })

    it('is empty (space-separated) works', () => {
      expect(
        evaluateFilter(row, { field: 'missing', operator: 'is empty' })
      ).toBe(true)
    })

    it('is not empty (space-separated) works', () => {
      expect(
        evaluateFilter(row, { field: 'name', operator: 'is not empty' })
      ).toBe(true)
    })
  })

  describe('not modifier', () => {
    it('negates a matching filter', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'contains',
          value: 'hello',
          not: true,
        })
      ).toBe(false)
    })

    it('negates a non-matching filter', () => {
      expect(
        evaluateFilter(row, {
          field: 'name',
          operator: 'contains',
          value: 'xyz',
          not: true,
        })
      ).toBe(true)
    })
  })

  describe('null/undefined field values', () => {
    it('returns false for null field with non-empty operator', () => {
      expect(
        evaluateFilter(row, {
          field: 'missing',
          operator: 'contains',
          value: 'x',
        })
      ).toBe(false)
    })

    it('returns true for null field with not modifier', () => {
      expect(
        evaluateFilter(row, {
          field: 'missing',
          operator: 'contains',
          value: 'x',
          not: true,
        })
      ).toBe(true)
    })
  })

  describe('legacy v5 property names', () => {
    it('reads columnField when field is absent', () => {
      expect(
        evaluateFilter(row, {
          columnField: 'name',
          operatorValue: 'contains',
          value: 'hello',
        })
      ).toBe(true)
    })

    it('reads operatorValue when operator is absent', () => {
      expect(
        evaluateFilter(row, {
          columnField: 'name',
          operatorValue: 'equals',
          value: 'hello world',
        })
      ).toBe(true)
    })
  })

  describe('valueGetter support', () => {
    it('uses valueGetter from column definition', () => {
      const columns = [
        {
          field: 'name',
          valueGetter: (value) => value.toUpperCase(),
        },
      ]
      expect(
        evaluateFilter(
          row,
          { field: 'name', operator: 'contains', value: 'HELLO' },
          columns
        )
      ).toBe(true)
    })
  })
})

describe('applyFilterModel', () => {
  const rows = [
    { name: 'foo bar', status: 'pass', count: 10 },
    { name: 'foo baz', status: 'fail', count: 20 },
    { name: 'qux', status: 'pass', count: 30 },
  ]

  it('returns all rows when filterModel is null', () => {
    expect(applyFilterModel(rows, null)).toEqual(rows)
  })

  it('returns all rows when filterModel has no items', () => {
    expect(applyFilterModel(rows, { items: [] })).toEqual(rows)
  })

  it('returns all rows when all filter items are empty', () => {
    expect(
      applyFilterModel(rows, {
        items: [{ field: 'name', operator: 'contains', value: '' }],
      })
    ).toEqual(rows)
  })

  it('filters with AND logic (default)', () => {
    const result = applyFilterModel(rows, {
      items: [
        { field: 'name', operator: 'contains', value: 'foo' },
        { field: 'status', operator: 'equals', value: 'pass' },
      ],
      logicOperator: 'and',
    })
    expect(result).toHaveLength(1)
    expect(result[0].name).toBe('foo bar')
  })

  it('filters with OR logic', () => {
    const result = applyFilterModel(rows, {
      items: [
        { field: 'name', operator: 'contains', value: 'qux' },
        { field: 'status', operator: 'equals', value: 'fail' },
      ],
      logicOperator: 'or',
    })
    expect(result).toHaveLength(2)
  })

  it('defaults to AND when logicOperator missing', () => {
    const result = applyFilterModel(rows, {
      items: [
        { field: 'name', operator: 'contains', value: 'foo' },
        { field: 'status', operator: 'equals', value: 'fail' },
      ],
    })
    expect(result).toHaveLength(1)
    expect(result[0].name).toBe('foo baz')
  })

  describe('legacy v5 filter names', () => {
    it('handles v5 columnField/operatorValue item names', () => {
      const result = applyFilterModel(rows, {
        items: [
          { columnField: 'name', operatorValue: 'contains', value: 'foo' },
        ],
      })
      expect(result).toHaveLength(2)
    })

    it('handles v5 linkOperator', () => {
      const result = applyFilterModel(rows, {
        items: [
          { columnField: 'name', operatorValue: 'contains', value: 'qux' },
          {
            columnField: 'status',
            operatorValue: 'equals',
            value: 'fail',
          },
        ],
        linkOperator: 'or',
      })
      expect(result).toHaveLength(2)
    })

    it('does not return empty results for legacy-named filters', () => {
      const result = applyFilterModel(rows, {
        items: [
          {
            columnField: 'name',
            operatorValue: 'contains',
            value: 'foo',
          },
        ],
        linkOperator: 'and',
      })
      expect(result).toHaveLength(2)
      expect(result[0].name).toBe('foo bar')
      expect(result[1].name).toBe('foo baz')
    })
  })
})
