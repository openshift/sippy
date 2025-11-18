import './GridToolbarFilterMenu.css'
import { Add, FilterList } from '@mui/icons-material'
import {
  Badge,
  Button,
  Fab,
  FormControl,
  Grid,
  InputLabel,
  MenuItem,
  Popover,
  Select,
  Tooltip,
} from '@mui/material'
import { filterTooltip } from './utils'
import { makeStyles } from '@mui/styles'
import Divider from '@mui/material/Divider'
import GridToolbarFilterItem, {
  operatorWithoutValue,
} from './GridToolbarFilterItem'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

const useStyles = makeStyles((theme) => ({
  filterMenu: {
    padding: 20,
  },
  filterAdd: {
    padding: 20,
    textAlign: 'right',
  },
  selector: {
    margin: theme.spacing(1),
    minWidth: 120,
  },
  resetButton: {
    marginLeft: 10,
  },
}))

/**
 * GridToolbarFilterMenu is a drop-in replacement for the built-in material data tables
 * filters. In the MIT licensed version, they only permit a single filter on a table. Our
 * component can do multiple filters, as well as adding the concept of a "not" modifier.
 */
export default function GridToolbarFilterMenu(props) {
  const classes = useStyles()
  const [anchorEl, setAnchorEl] = React.useState(null)
  const [models, setModels] = React.useState(props.filterModel.items || [])

  const [linkOperator, setLinkOperator] = React.useState(
    props.filterModel.linkOperator || 'and'
  )

  useEffect(() => {
    if (props.filterModel.items !== models.items) {
      setModels(props.filterModel.items)
    }

    if (models.length === 0) {
      addFilter()
    }
  }, [props, models])

  // Ensure columns are ordered alphabetically
  const orderedColumns = [...props.columns]
  orderedColumns.sort((a, b) => {
    if (a.field > b.field) {
      return 1
    } else if (b.field > a.field) {
      return -1
    } else {
      return 0
    }
  })

  const handleClick = (event) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    let errored = 0
    let newModels = []

    models.map((m, index) => {
      if (m.errors && m.errors.length > 0) {
        errored++
      }

      if (!(m.operatorValue === '' && m.columnField === '' && m.value === '')) {
        newModels.push(m)
      }
    })

    if (errored === 0) {
      props.setFilterModel({
        items: [...newModels],
        linkOperator: linkOperator,
      })
      setModels(newModels)
      setAnchorEl(null)
    }
  }

  const open = Boolean(anchorEl)
  const id = open ? 'filter-popover' : undefined

  const addFilter = () => {
    let currentFilters = models

    currentFilters.push({
      id: models.length + 1,
      columnField: '',
      operatorValue: '',
      value: '',
    })

    setModels([...currentFilters])
  }

  const removeFilter = (index) => {
    let currentFilters = models

    if (currentFilters.length === 1) {
      currentFilters[index] = {
        columnField: '',
        operatorValue: '',
        value: '',
      }
    } else {
      currentFilters.splice(index, 1)
    }

    setModels([...currentFilters])
  }

  const updateModel = (index, v) => {
    const fields = ['columnField', 'operatorValue', 'value']
    const blankFields = fields
      .map((field) => {
        if (
          !operatorWithoutValue.includes(v.operatorValue) &&
          !v[field] &&
          v[field] === ''
        ) {
          return field
        } else {
          return null
        }
      })
      .filter((field) => field)

    if (blankFields.length > 0 && blankFields.length < 3) {
      v.errors = blankFields
    }

    let columnType = 'string'
    props.columns.forEach((col) => {
      if (col.field === v.columnField) {
        columnType = col.type || 'string'
      }
    })
    if (columnType === 'number' && isNaN(v.value)) {
      v.errors = v.errors || []
      v.errors.includes('value') || v.errors.push('value')
    }

    let currentModels = models
    currentModels[index] = v
    setModels([...currentModels])
  }

  let filterItems =
    props.filterModel.items.length === 1 &&
    props.filterModel.items[0].value === ''
      ? 0
      : props.filterModel.items.length

  const linkOperatorForm = (
    <FormControl variant="standard">
      <InputLabel id="linkOperatorLabel">Link operator</InputLabel>
      <Select
        variant="standard"
        value={linkOperator}
        onChange={(e) => setLinkOperator(e.target.value)}
        className={classes.selector}
        labelId="linkOperatorLabel"
        id="linkOperator"
        autoWidth
      >
        <MenuItem value="and">and</MenuItem>
        <MenuItem value="or">or</MenuItem>
      </Select>
    </FormControl>
  )

  return (
    <Fragment>
      <Tooltip title={filterTooltip(props.filterModel)}>
        <Button
          aria-describedby={id}
          color="primary"
          variant={props.standalone ? 'contained' : 'text'}
          onClick={handleClick}
        >
          <Badge badgeContent={filterItems} color="primary">
            <FilterList />
          </Badge>
          Filters
        </Button>
      </Tooltip>

      <Popover
        id="filter-popover"
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
      >
        <div className={classes.filterMenu}>
          {models.map((item, index) => {
            return (
              <div key={`filter-item-${index}`} style={{ paddingTop: 20 }}>
                <GridToolbarFilterItem
                  id={index}
                  columns={orderedColumns}
                  destroy={() => removeFilter(index)}
                  filterModel={models[index]}
                  setFilterModel={(v) => updateModel(index, v)}
                  autocompleteData={props.autocompleteData}
                />
                <Divider />
              </div>
            )
          })}
        </div>

        <Grid
          container
          justifyContent="space-between"
          className={classes.filterAdd}
        >
          <Fab
            data-testid="add-button"
            onClick={addFilter}
            size="small"
            color="secondary"
            aria-label="add"
          >
            <Add />
          </Fab>
          {models.length > 1 && !props.linkOperatorDisabled
            ? linkOperatorForm
            : ''}
          <Button variant="contained" color="primary" onClick={handleClose}>
            Filter
          </Button>
        </Grid>
      </Popover>
    </Fragment>
  )
}

GridToolbarFilterItem.defaultProps = {
  standalone: false,
  linkOperatorDisabled: false,
}

GridToolbarFilterMenu.propTypes = {
  linkOperatorDisabled: PropTypes.bool,
  standalone: PropTypes.bool,
  setFilterModel: PropTypes.func.isRequired,
  filterModel: PropTypes.shape({
    items: PropTypes.arrayOf(
      PropTypes.shape({
        columnField: PropTypes.string,
        not: PropTypes.bool,
        operatorValue: PropTypes.string,
        value: PropTypes.string,
      })
    ).isRequired,
    linkOperator: PropTypes.string,
  }),
  columns: PropTypes.arrayOf(
    PropTypes.shape({
      field: PropTypes.string.isRequired,
      headerName: PropTypes.string,
      type: PropTypes.string,
    })
  ),
  autocompleteData: PropTypes.array,
}
