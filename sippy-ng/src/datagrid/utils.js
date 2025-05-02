import { Close } from '@mui/icons-material'
import { createTheme } from '@mui/material/styles'
import { format, utcToZonedTime } from 'date-fns-tz'
import { green, orange, red } from '@mui/material/colors'
import { Link } from 'react-router-dom'
import { ListItem, ListItemIcon, ListItemText, Tooltip } from '@mui/material'
import { scale } from 'chroma-js'
import List from '@mui/material/List'
import React, { Fragment } from 'react'

// TODO(v5): colors aren't right but I can't wire in the provider's theme yet here...
const theme = createTheme({
  palette: {
    mode: 'light',
    success: {
      main: green[500],
      light: green[300],
      dark: green[700],
    },
    warning: {
      main: orange[500],
      light: orange[300],
      dark: orange[700],
    },
    error: {
      main: red[500],
      light: red[300],
      dark: red[700],
    },
  },
})

// DataGrid tables can only be customized by specifying a classname, so this
// creates the classes needed for creating background color gradient.
export function generateClasses(threshold, invert = false) {
  let thresholds = [threshold.error, threshold.warning, threshold.success]
  let scaleColors = [
    theme.palette.error.light,
    theme.palette.warning.light,
    theme.palette.success.light,
  ]
  if (invert) {
    let scaleColors = [
      theme.palette.success.light,
      theme.palette.warning.light,
      theme.palette.error.light,
    ]
  }
  const colors = scale(scaleColors).domain(thresholds)

  const classes = {}

  for (let i = 0; i <= 100; i++) {
    classes['row-percent-' + i] = { backgroundColor: colors(i).hex() }
  }

  classes['overall'] = {
    filter: 'brightness(80%) contrast(150%)',
    borderBottom: '5px solid black',
  }

  return classes
}

export function filterRemoveItem(filter, index) {
  if (!filter || filter.items.length === 0) {
    return
  }
  let currentItems = filter.items
  currentItems.splice(index, 1)
  return {
    items: currentItems,
    linkOperator: filter.linkOperator,
  }
}

export function filterIsEmpty(filter) {
  return (
    !filter ||
    filter.items.length === 0 ||
    (filter.items.length === 1 && filter.items[0].columnField === '')
  )
}

export function filterItemRenderValue(item) {
  let value = item.value
  let tooltip = null
  if (item.columnField === 'timestamp' && item.value !== '') {
    let date = new Date(parseInt(item.value))
    value = format(utcToZonedTime(date, 'UTC'), "yyyy-MM-dd HH:mm 'UTC'", {
      timeZone: 'Etc/UTC',
    })
    tooltip = date.toLocaleString()
  }

  return { value: value, description: tooltip }
}

export function filterTooltip(filter) {
  if (filterIsEmpty(filter)) {
    return 'Showing all results'
  }

  const items = filter.items.map((item, index) => {
    let { value, description } = filterItemRenderValue(item)

    return (
      <li key={`filter-${index}`}>
        {`${item.columnField}${item.not ? ' not ' : ' '}${
          item.operatorValue
        } ${value}`}
        {description ? `(${description})` : ''}
      </li>
    )
  })

  return (
    <Fragment>
      Current filters:
      <ul>{items}</ul>
      Link operator: {filter.linkOperator}
    </Fragment>
  )
}

export function filterList(filter, setFilter) {
  let originalFilters = filter
  if (filterIsEmpty(filter)) {
    return 'Showing all results'
  }

  const explanations = []
  filter.items.forEach((item, idx) => {
    let { value, description } = filterItemRenderValue(item)
    let listItem = (
      <ListItemText>
        {`${item.columnField} ${item.not ? 'not ' : ''} ${item.operatorValue} `}
        {value}
      </ListItemText>
    )

    if (description && description !== '') {
      listItem = <Tooltip title={description}>{listItem}</Tooltip>
    }

    explanations.push(
      <ListItem
        key={`filter-item-${idx}`}
        onClick={
          setFilter
            ? () => setFilter(filterRemoveItem(originalFilters, idx))
            : null
        }
      >
        {setFilter ? (
          <ListItemIcon component={Link}>
            <Close />
          </ListItemIcon>
        ) : (
          ''
        )}
        {listItem}
      </ListItem>
    )
  })

  return <List>{explanations}</List>
}
