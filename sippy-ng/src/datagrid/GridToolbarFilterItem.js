import {
  Button,
  Checkbox,
  FormControl,
  FormHelperText,
  Grid,
  InputLabel,
  Menu,
  MenuItem,
  Select,
  TextField,
} from '@material-ui/core'
import { Close } from '@material-ui/icons'
import { DateTimePicker, MuiPickersUtilsProvider } from '@material-ui/pickers'
import { GridToolbarFilterDateUtils } from './GridToolbarFilterDateUtils'
import { makeStyles } from '@material-ui/core/styles'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

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
}))

const operatorValues = {
  number: ['=', '!=', '<', '<=', '>', '>='],
  date: ['=', '!=', '<', '<=', '>', '>='],
  string: ['contains', 'equals', 'starts with', 'ends with'],
  array: ['contains'],
}

/**
 * GridToolbarFilterItem represents a single filter used by GridToolbarFilterMenu, consisting
 * of a column field, operator, value, and optional not modifier.
 */
export default function GridToolbarFilterItem(props) {
  const classes = useStyles()

  let columnType = 'string'
  let enums = {}
  props.columns.forEach((col) => {
    if (col.field === props.filterModel.columnField) {
      columnType = col.type || 'string'
      enums = col.enums || enums
    }
  })

  const updateColumnField = (e) => {
    props.setFilterModel({
      columnField: e.target.value,
      operatorValue: '',
      value: '',
    })
  }

  const columnFieldError =
    props.filterModel.errors && props.filterModel.errors.includes('columnField')
  const operatorValueError =
    props.filterModel.errors &&
    props.filterModel.errors.includes('operatorValue')
  const valueError =
    props.filterModel.errors && props.filterModel.errors.includes('value')

  const inputField = (columnType) => {
    if (columnType === 'date') {
      return (
        <Fragment>
          <MuiPickersUtilsProvider utils={GridToolbarFilterDateUtils}>
            <DateTimePicker
              showTodayButton
              disableFuture
              label="Value"
              format="yyyy-MM-dd HH:mm 'UTC'"
              ampm={false}
              value={
                props.filterModel.value === ''
                  ? null
                  : new Date(parseInt(props.filterModel.value))
              }
              onChange={(e) => {
                props.setFilterModel({
                  columnField: props.filterModel.columnField,
                  operatorValue: props.filterModel.operatorValue,
                  value: e.getTime().toString(),
                })
              }}
            />
          </MuiPickersUtilsProvider>
        </Fragment>
      )
    } else if (Object.keys(enums).length !== 0) {
      return (
        <Fragment>
          <InputLabel id={`value-${props.id}`}>Value</InputLabel>
          <Select
            autoWidth
            labelId={`value-${props.id}`}
            id={`value-select-${props.id}`}
            value={props.filterModel.value}
            onChange={(e) =>
              props.setFilterModel({
                columnField: props.filterModel.columnField,
                not: props.filterModel.not,
                operatorValue: props.filterModel.operatorValue,
                value: e.target.value,
              })
            }
          >
            {Object.keys(enums).map((k, i) => (
              <MenuItem key={`menu-value-${props.id}-${i}`} value={k}>
                {enums[k]}
              </MenuItem>
            ))}
          </Select>
        </Fragment>
      )
    } else {
      return (
        <Fragment>
          <TextField
            inputProps={{ 'data-testid': `value-${props.id}` }}
            error={operatorValueError}
            id="value"
            label="Value"
            onChange={(e) =>
              props.setFilterModel({
                columnField: props.filterModel.columnField,
                not: props.filterModel.not,
                operatorValue: props.filterModel.operatorValue,
                value: e.target.value,
              })
            }
            value={props.filterModel.value}
          />
          <FormHelperText error={valueError} style={{ marginTop: 12 }}>
            {columnType === 'number' ? 'Numerical value required' : 'Required'}
          </FormHelperText>
        </Fragment>
      )
    }
  }

  return (
    <Grid container>
      <Button startIcon={<Close />} onClick={props.destroy} />
      <FormControl>
        <InputLabel id={`columnFieldLabel-${props.id}`}>Field</InputLabel>
        <Select
          inputProps={{ 'data-testid': `columnField-${props.id}` }}
          error={columnFieldError}
          value={props.filterModel.columnField}
          onChange={updateColumnField}
          className={classes.selector}
          labelId={`columnFieldLabel-${props.id}`}
          id={`columnField-${props.id}`}
          autoWidth
        >
          {props.columns
            .filter(
              (col) => col.filterable === undefined || col.filterable === true
            )
            .sort((a, b) =>
              a.field === b.field ? 0 : a.field < b.field ? -1 : 1
            )
            .map((col) => (
              <MenuItem key={col.field} value={col.field}>
                {col.headerName ? col.headerName : col.field}
              </MenuItem>
            ))}
        </Select>
        <FormHelperText error={columnFieldError}>Required</FormHelperText>
      </FormControl>
      <FormControl>
        <InputLabel shrink id={`notLabel-${props.id}`}>
          Not
        </InputLabel>
        <Checkbox
          inputProps={{ 'data-testid': `not-${props.id}` }}
          style={{ marginTop: 10 }}
          color={'primary'}
          checked={props.filterModel.not}
          onChange={(e) =>
            props.setFilterModel({
              columnField: props.filterModel.columnField,
              not: e.target.checked,
              operatorValue: props.filterModel.operatorValue,
              value: props.filterModel.value,
            })
          }
          aria-labelledby={`notLabel-${props.id}`}
        />
      </FormControl>
      <FormControl>
        <InputLabel id={`operatorValueLabel-${props.id}`}>Operator</InputLabel>
        <Select
          inputProps={{ 'data-testid': `operatorValue-${props.id}` }}
          error={operatorValueError}
          onChange={(e) =>
            props.setFilterModel({
              columnField: props.filterModel.columnField,
              not: props.filterModel.not,
              operatorValue: e.target.value,
              value: props.filterModel.value,
            })
          }
          value={props.filterModel.operatorValue}
          className={classes.selector}
          labelId={`operatorValueLabel-${props.id}`}
          id={`operatorValue-${props.id}`}
          autoWidth
        >
          {operatorValues[columnType].map((operator, index) => (
            <MenuItem key={'operator-' + index} value={operator}>
              {operator}
            </MenuItem>
          ))}
        </Select>
        <FormHelperText error={operatorValueError}>Required</FormHelperText>
      </FormControl>
      <FormControl style={{ minWidth: 240 }}>
        {inputField(columnType)}
      </FormControl>
    </Grid>
  )
}

GridToolbarFilterItem.defaultProps = {
  columns: [],
  errors: [],
}

GridToolbarFilterItem.propTypes = {
  id: PropTypes.number.isRequired,
  destroy: PropTypes.func,
  filterModel: PropTypes.object,
  setFilterModel: PropTypes.func,
  errors: PropTypes.array,
  columns: PropTypes.arrayOf(
    PropTypes.shape({
      field: PropTypes.string.isRequired,
      headerName: PropTypes.string,
      type: PropTypes.string,
    }).isRequired
  ),
}
