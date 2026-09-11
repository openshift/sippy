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
import { isValid } from 'date-fns'
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
export default function GridToolbarFilterItem({ columns = [], ...props }) {
  const classes = useStyles()

  let columnType = 'string'
  let autocomplete = ''
  let release = ''
  let disabled = false
  let valueGetter = null
  let values = null
  columns.forEach((col) => {
    if (col.field === props.filterModel.field) {
      columnType = col.type || 'string'
      autocomplete = col.autocomplete || ''
      release = col.release || ''
      disabled = col.disabled || false
      valueGetter = col.valueGetter || null
      values = col.values || null
    }
  })

  // Columns with a fixed set of "values" are rendered as a strict dropdown
  // instead of a free-text/autocomplete field, and only support equals/not-equals
  // since there's nothing meaningful to contain/start with/end with.
  const operators = values ? ['equals', '!='] : operatorValues[columnType]

  const updateField = (e) => {
    props.setFilterModel({
      field: e.target.value,
      operator: '',
      value: '',
    })
  }

  const fieldError =
    props.filterModel.errors && props.filterModel.errors.includes('field')
  const operatorError =
    props.filterModel.errors && props.filterModel.errors.includes('operator')
  const valueError =
    props.filterModel.errors && props.filterModel.errors.includes('value')

  const inputField = () => {
    if (
      props.filterModel.operator === 'is empty' ||
      props.filterModel.operator === 'is not empty'
    ) {
      return ''
    }

    if (values) {
      return (
        <Fragment>
          <InputLabel id={`valueLabel-${props.id}`}>Value</InputLabel>
          <Select
            variant="standard"
            disabled={disabled}
            inputProps={{ 'data-testid': `value-${props.id}` }}
            error={valueError}
            value={props.filterModel.value}
            onChange={(e) =>
              props.setFilterModel({
                field: props.filterModel.field,
                not: props.filterModel.not,
                operator: props.filterModel.operator,
                value: e.target.value,
              })
            }
            className={classes.selector}
            labelId={`valueLabel-${props.id}`}
            id={`value-${props.id}`}
            autoWidth
          >
            {values.map((value) => (
              <MenuItem key={value} value={value}>
                {value}
              </MenuItem>
            ))}
          </Select>
          <FormHelperText error={valueError}>Required</FormHelperText>
        </Fragment>
      )
    }

    switch (columnType) {
      case 'date':
        return (
          <Fragment>
            <LocalizationProvider dateAdapter={AdapterDateFns}>
              <DateTimePicker
                disabled={disabled}
                disableFuture
                label="Value"
                format="yyyy-MM-dd HH:mm 'UTC'"
                ampm={false}
                value={
                  props.filterModel.value === ''
                    ? null
                    : new Date(props.filterModel.value)
                }
                onChange={(e) => {
                  if (e && isValid(e)) {
                    props.setFilterModel({
                      field: props.filterModel.field,
                      not: props.filterModel.not,
                      operator: props.filterModel.operator,
                      value: e.toISOString(),
                    })
                  }
                }}
                slotProps={{ textField: { variant: 'standard' } }}
              />
            </LocalizationProvider>
            <FormHelperText error={operatorError}>Required</FormHelperText>
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
                field={props.filterModel.field}
                id={`value-${props.id}`}
                label="Value"
                value={props.filterModel.value}
                data={props.autocompleteData}
                valueGetter={valueGetter}
                onChange={(value) =>
                  props.setFilterModel({
                    field: props.filterModel.field,
                    not: props.filterModel.not,
                    operator: props.filterModel.operator,
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
                    field: props.filterModel.field,
                    not: props.filterModel.not,
                    operator: props.filterModel.operator,
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
                error={operatorError}
                id={`value-${props.id}`}
                label="Value"
                onChange={(e) =>
                  props.setFilterModel({
                    field: props.filterModel.field,
                    not: props.filterModel.not,
                    operator: props.filterModel.operator,
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
        <InputLabel id={`fieldLabel-${props.id}`}>Field</InputLabel>
        <Select
          variant="standard"
          disabled={disabled}
          inputProps={{ 'data-testid': `field-${props.id}` }}
          error={fieldError}
          value={props.filterModel.field}
          onChange={updateField}
          className={classes.selector}
          labelId={`fieldLabel-${props.id}`}
          id={`field-${props.id}`}
          autoWidth
        >
          {columns
            .filter(
              (col) => col.filterable === undefined || col.filterable === true
            )
            .map((col) =>
              col.disabled &&
              props.filterModel.field !== col.field ? undefined : (
                <MenuItem key={col.field} value={col.field}>
                  {col.headerName ? col.headerName : col.field}
                </MenuItem>
              )
            )}
        </Select>
        <FormHelperText error={fieldError}>Required</FormHelperText>
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
              field: props.filterModel.field,
              not: e.target.checked,
              operator: props.filterModel.operator,
              value: props.filterModel.value,
            })
          }
          aria-labelledby={`notLabel-${props.id}`}
        />
      </FormControl>
      <FormControl variant="standard">
        <InputLabel id={`operatorLabel-${props.id}`}>Operator</InputLabel>
        <Select
          variant="standard"
          disabled={disabled}
          inputProps={{ 'data-testid': `operator-${props.id}` }}
          error={operatorError}
          onChange={(e) =>
            props.setFilterModel({
              field: props.filterModel.field,
              not: props.filterModel.not,
              operator: e.target.value,
              value: props.filterModel.value,
            })
          }
          value={props.filterModel.operator}
          className={classes.selector}
          labelId={`operatorLabel-${props.id}`}
          id={`operator-${props.id}`}
          autoWidth
        >
          {operators.map((operator, index) => (
            <MenuItem key={'operator-' + index} value={operator}>
              {operator}
            </MenuItem>
          ))}
        </Select>
        <FormHelperText error={operatorError}>Required</FormHelperText>
      </FormControl>
      <FormControl variant="standard">{inputField()}</FormControl>
    </Grid>
  )
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
      values: PropTypes.arrayOf(PropTypes.string),
    }).isRequired
  ),
  autocompleteData: PropTypes.array,
}
