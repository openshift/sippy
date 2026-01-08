import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns'
import {
  Button,
  Checkbox,
  FormControl,
  FormHelperText,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  TextField,
} from '@mui/material'
import { Close } from '@mui/icons-material'
import { DateTimePicker, LocalizationProvider } from '@mui/x-date-pickers'
import { makeStyles } from '@mui/styles'
import GridToolbarAutocomplete from './GridToolbarAutocomplete'
import GridToolbarClientAutocomplete from './GridToolbarClientAutocomplete'
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

export const operatorWithoutValue = ['is empty', 'is not empty']

const operatorValues = {
  number: ['=', '!=', '<', '<=', '>', '>=', 'is empty', 'is not empty'],
  date: ['=', '!=', '<', '<=', '>', '>=', 'is empty', 'is not empty'],
  string: ['contains', 'equals', 'starts with', 'ends with'],
  array: ['has entry', 'has entry containing', 'is empty'],
}

/**
 * GridToolbarFilterItem represents a single filter used by GridToolbarFilterMenu, consisting
 * of a column field, operator, value, and optional not modifier.
 */
export default function GridToolbarFilterItem(props) {
  const classes = useStyles()

  let columnType = 'string'
  let autocomplete = ''
  let release = ''
  let disabled = false
  let valueGetter = null
  props.columns.forEach((col) => {
    if (col.field === props.filterModel.columnField) {
      columnType = col.type || 'string'
      autocomplete = col.autocomplete || ''
      release = col.release || ''
      disabled = col.disabled || false
      valueGetter = col.valueGetter || null
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

  const inputField = () => {
    if (
      props.filterModel.operatorValue === 'is empty' ||
      props.filterModel.operatorValue === 'is not empty'
    ) {
      return ''
    }

    switch (columnType) {
      case 'date':
        return (
          <Fragment>
            <LocalizationProvider dateAdapter={AdapterDateFns}>
              <DateTimePicker
                disabled={disabled}
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
                  if (e && e.getTime()) {
                    props.setFilterModel({
                      columnField: props.filterModel.columnField,
                      operatorValue: props.filterModel.operatorValue,
                      value: e.getTime().toString(),
                    })
                  }
                }}
                renderInput={(props) => (
                  <TextField variant="standard" {...props} />
                )}
              />
            </LocalizationProvider>
            <FormHelperText error={operatorValueError}>Required</FormHelperText>
          </Fragment>
        )
      default:
        if (autocomplete !== '') {
          // Use client-side autocomplete if data is available, otherwise use server-side
          if (props.autocompleteData && props.autocompleteData.length > 0) {
            return (
              <GridToolbarClientAutocomplete
                error={valueError}
                disabled={disabled}
                field={props.filterModel.columnField}
                id={`value-${props.id}`}
                label="Value"
                value={props.filterModel.value}
                data={props.autocompleteData}
                valueGetter={valueGetter}
                onChange={(value) =>
                  props.setFilterModel({
                    columnField: props.filterModel.columnField,
                    not: props.filterModel.not,
                    operatorValue: props.filterModel.operatorValue,
                    value: value,
                  })
                }
              />
            )
          } else {
            return (
              <GridToolbarAutocomplete
                error={valueError}
                disabled={disabled}
                field={autocomplete}
                id={`value-${props.id}`}
                label="Value"
                value={props.filterModel.value}
                release={release}
                onChange={(value) =>
                  props.setFilterModel({
                    columnField: props.filterModel.columnField,
                    not: props.filterModel.not,
                    operatorValue: props.filterModel.operatorValue,
                    value: value,
                  })
                }
              />
            )
          }
        } else {
          return (
            <Fragment>
              <TextField
                variant="standard"
                disabled={disabled}
                inputProps={{ 'data-testid': `value-${props.id}` }}
                error={operatorValueError}
                id={`value-${props.id}`}
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
                {columnType === 'number'
                  ? 'Numerical value required'
                  : 'Required'}
              </FormHelperText>
            </Fragment>
          )
        }
    }
  }

  return (
    <Grid container>
      {disabled ? (
        <Button disabled />
      ) : (
        <Button startIcon={<Close />} onClick={props.destroy} />
      )}
      <FormControl variant="standard">
        <InputLabel id={`columnFieldLabel-${props.id}`}>Field</InputLabel>
        <Select
          variant="standard"
          disabled={disabled}
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
            .map((col) =>
              col.disabled &&
              props.filterModel.columnField !== col.field ? undefined : (
                <MenuItem key={col.field} value={col.field}>
                  {col.headerName ? col.headerName : col.field}
                </MenuItem>
              )
            )}
        </Select>
        <FormHelperText error={columnFieldError}>Required</FormHelperText>
      </FormControl>
      <FormControl variant="standard">
        <InputLabel shrink id={`notLabel-${props.id}`}>
          Not
        </InputLabel>
        <Checkbox
          disabled={disabled}
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
      <FormControl variant="standard">
        <InputLabel id={`operatorValueLabel-${props.id}`}>Operator</InputLabel>
        <Select
          variant="standard"
          disabled={disabled}
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
      <FormControl variant="standard">{inputField()}</FormControl>
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
      autocomplete: PropTypes.string,
      release: PropTypes.string,
      disabled: PropTypes.bool,
    }).isRequired
  ),
  autocompleteData: PropTypes.array,
}
