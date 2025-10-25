/**
 * Client-side filtering utilities for DataGrid components.
 *
 * These utilities support Sippy's custom filter model which allows:
 * - Multiple filters with AND/OR operators
 * - NOT modifier for negating filters
 * - Various filter operators (contains, equals, startsWith, endsWith, comparison operators)
 */

/**
 * Applies a filter model to an array of rows
 *
 * @param {Array} rows - The data rows to filter
 * @param {Object} filterModel - The filter model with items and linkOperator
 * @param {Array} filterModel.items - Array of filter items
 * @param {string} filterModel.linkOperator - 'and' or 'or' to combine filters
 * @param {Array} columns - Optional column definitions with valueGetters
 * @returns {Array} Filtered rows
 */
export function applyFilterModel(rows, filterModel, columns = null) {
  if (!filterModel || !filterModel.items || filterModel.items.length === 0) {
    return rows
  }

  return rows.filter((row) => {
    const results = filterModel.items.map((filter) =>
      evaluateFilter(row, filter, columns)
    )

    // Apply AND/OR logic
    const linkOperator = filterModel.linkOperator || 'and'
    return linkOperator === 'and'
      ? results.every((r) => r)
      : results.some((r) => r)
  })
}

/**
 * Evaluates a single filter against a row
 *
 * @param {Object} row - The data row
 * @param {Object} filter - The filter to apply
 * @param {string} filter.columnField - The field name to filter on
 * @param {string} filter.operatorValue - The comparison operator
 * @param {any} filter.value - The value to compare against
 * @param {boolean} filter.not - Whether to negate the result
 * @param {Array} columns - Optional column definitions with valueGetters
 * @returns {boolean} Whether the row matches the filter
 */
export function evaluateFilter(row, filter, columns = null) {
  let fieldValue = row[filter.columnField]

  // Use valueGetter if available in columns definition
  if (columns) {
    const column = columns.find((col) => col.field === filter.columnField)
    if (column && column.valueGetter) {
      fieldValue = column.valueGetter({ row, value: fieldValue })
    }
  }

  let match = false

  // Handle null/undefined/empty values for isEmpty/isNotEmpty operators
  const isNullOrUndefined = fieldValue === null || fieldValue === undefined
  const isEmpty = isNullOrUndefined || fieldValue === ''

  // For isEmpty/isNotEmpty, treat null/undefined as empty
  if (
    filter.operatorValue === 'isEmpty' ||
    filter.operatorValue === 'is empty'
  ) {
    match = isEmpty
    return filter.not ? !match : match
  }
  if (
    filter.operatorValue === 'isNotEmpty' ||
    filter.operatorValue === 'is not empty'
  ) {
    match = !isEmpty
    return filter.not ? !match : match
  }

  // For other operators, null/undefined means no match
  if (isNullOrUndefined) {
    return filter.not ? true : false
  }

  const value = String(fieldValue).toLowerCase()
  const filterValue = String(filter.value).toLowerCase()

  switch (filter.operatorValue) {
    case 'contains':
      match = value.includes(filterValue)
      break
    case 'equals':
      match = value === filterValue
      break
    case 'startsWith':
      match = value.startsWith(filterValue)
      break
    case 'endsWith':
      match = value.endsWith(filterValue)
      break
    case '>':
      match = parseFloat(fieldValue) > parseFloat(filter.value)
      break
    case '>=':
      match = parseFloat(fieldValue) >= parseFloat(filter.value)
      break
    case '<':
      match = parseFloat(fieldValue) < parseFloat(filter.value)
      break
    case '<=':
      match = parseFloat(fieldValue) <= parseFloat(filter.value)
      break
    case '!=':
    case 'not equals':
      match = value !== filterValue
      break
    default:
      // Default to contains for unknown operators
      match = value.includes(filterValue)
  }

  return filter.not ? !match : match
}

/**
 * React hook for managing filtered data with a filter model
 *
 * @param {Array} data - The data to filter
 * @param {Object} filterModel - The filter model
 * @returns {Array} Filtered data
 *
 * @example
 * const filteredRows = useFilteredData(rows, filterModel)
 */
export function useFilteredData(data, filterModel) {
  const React = require('react')
  return React.useMemo(
    () => applyFilterModel(data, filterModel),
    [data, filterModel]
  )
}
