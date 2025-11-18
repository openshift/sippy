# Client-Side Filtering Utilities

This module provides client-side filtering utilities for DataGrid components that work with Sippy's custom filter model.

## Why This Exists

MUI DataGrid's built-in client-side filtering only supports single filters. Sippy's filter system is more advanced, supporting:
- **Multiple filters** with AND/OR operators
- **NOT modifier** for negating filters
- **Various filter operators** (contains, equals, startsWith, endsWith, comparison operators)

For components that work with pre-loaded data (like modals), we need to manually filter the data client-side.

## Usage

### Basic Example

```javascript
import { applyFilterModel } from '../datagrid/filterUtils'
import React from 'react'

function MyComponent({ data, filterModel }) {
  // Filter the data client-side
  const filteredData = React.useMemo(
    () => applyFilterModel(data, filterModel),
    [data, filterModel]
  )

  return <DataGrid rows={filteredData} {...otherProps} />
}
```

### Complete Example with Query Parameters

```javascript
import { applyFilterModel } from '../datagrid/filterUtils'
import { DataGrid } from '@mui/x-data-grid'
import { SafeJSONParam } from '../helpers'
import { useQueryParam } from 'use-query-params'
import GridToolbar from '../datagrid/GridToolbar'
import React from 'react'

export default function MyFilterableComponent({ data }) {
  // Manage filter state in URL query parameters
  const [filterModel = { items: [] }, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  // Add filters from bookmarks or other sources
  const addFilters = (filter) => {
    const currentFilters = filterModel.items.filter((item) => item.value !== '')
    filter.forEach((item) => {
      if (item.value && item.value !== '') {
        currentFilters.push(item)
      }
    })
    setFilterModel({
      items: currentFilters,
      linkOperator: filterModel.linkOperator || 'and',
    })
  }

  // Apply client-side filtering
  const filteredData = React.useMemo(
    () => applyFilterModel(data, filterModel),
    [data, filterModel]
  )

  return (
    <DataGrid
      rows={filteredData}
      columns={columns}
      components={{ Toolbar: GridToolbar }}
      componentsProps={{
        toolbar: {
          columns: columns,
          addFilters: addFilters,
          filterModel: filterModel,
          setFilterModel: setFilterModel,
          clearSearch: () => {},
          doSearch: () => {},
        },
      }}
    />
  )
}
```

## API

### `applyFilterModel(rows, filterModel)`

Applies a filter model to an array of rows.

**Parameters:**
- `rows` (Array): The data rows to filter
- `filterModel` (Object): The filter model with `items` and `linkOperator`
  - `items` (Array): Array of filter items
  - `linkOperator` (string): 'and' or 'or' to combine filters

**Returns:** Array of filtered rows

### `evaluateFilter(row, filter)`

Evaluates a single filter against a row.

**Parameters:**
- `row` (Object): The data row
- `filter` (Object): The filter to apply
  - `columnField` (string): The field name to filter on
  - `operatorValue` (string): The comparison operator
  - `value` (any): The value to compare against
  - `not` (boolean): Whether to negate the result

**Returns:** Boolean indicating whether the row matches the filter

## Supported Filter Operators

- `contains`: Case-insensitive substring match
- `equals`: Exact match (case-insensitive)
- `startsWith`: Starts with (case-insensitive)
- `endsWith`: Ends with (case-insensitive)
- `isEmpty` / `is empty`: Field is empty string
- `isNotEmpty` / `is not empty`: Field is not empty
- `>`: Greater than (numeric)
- `>=`: Greater than or equal (numeric)
- `<`: Less than (numeric)
- `<=`: Less than or equal (numeric)
- `!=` / `not equals`: Not equal to

## Filter Model Structure

```javascript
{
  items: [
    {
      columnField: 'component',
      operatorValue: 'contains',
      value: 'storage',
      not: false
    },
    {
      columnField: 'status',
      operatorValue: 'equals',
      value: 'regressed',
      not: false
    }
  ],
  linkOperator: 'and' // or 'or'
}
```

## Performance

The `applyFilterModel` function uses Array.filter which is O(n). For large datasets, use it with `React.useMemo` to avoid unnecessary recomputations:

```javascript
const filteredData = React.useMemo(
  () => applyFilterModel(data, filterModel),
  [data, filterModel]
)
```

This ensures filtering only happens when `data` or `filterModel` changes.

