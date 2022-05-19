import { Close } from '@material-ui/icons'
import {
  createTheme,
  ListItem,
  ListItemIcon,
  ListItemText,
  Tooltip,
} from '@material-ui/core'
import { format, utcToZonedTime } from 'date-fns-tz'
import { Link } from 'react-router-dom'
import { scale } from 'chroma-js'
import List from '@material-ui/core/List'
import React, { Fragment } from 'react'

const theme = createTheme()

// DataGrid tables can only be customized by specifying a classname, so this
// creates the classes needed for creating background color gradient.
export function generateClasses(threshold) {
  const colors = scale([
    theme.palette.error.light,
    theme.palette.warning.light,
    theme.palette.success.light,
  ]).domain([threshold.error, threshold.warning, threshold.success])

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
        button={Boolean(setFilter)}
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
