import { Close } from '@mui/icons-material'
import { format, utcToZonedTime } from 'date-fns-tz'
import { Link } from 'react-router'
import { ListItem, ListItemIcon, ListItemText, Tooltip } from '@mui/material'
import List from '@mui/material/List'
import React, { Fragment } from 'react'

export function filterRemoveItem(filter, index) {
  if (!filter || filter.items.length === 0) {
    return
  }
  return {
    items: filter.items.filter((_, i) => i !== index),
    logicOperator: filter.logicOperator,
  }
}

export function filterIsEmpty(filter) {
  return (
    !filter ||
    filter.items.length === 0 ||
    (filter.items.length === 1 && (filter.items[0].field || '') === '')
  )
}

export function filterItemRenderValue(item) {
  let value = item.value
  let tooltip = null
  const itemField = item.field
  if (itemField === 'timestamp' && item.value !== '') {
    let date = new Date(item.value)
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
        {`${item.field}${item.not ? ' not ' : ' '}${item.operator} ${value}`}
        {description ? `(${description})` : ''}
      </li>
    )
  })

  return (
    <Fragment>
      Current filters:
      <ul>{items}</ul>
      Link operator: {filter.logicOperator}
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
        {`${item.field} ${item.not ? 'not ' : ''} ${item.operator} `}
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
