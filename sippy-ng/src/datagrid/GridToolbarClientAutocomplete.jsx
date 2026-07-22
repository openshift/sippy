import Autocomplete from '@mui/lab/Autocomplete'
import PropTypes from 'prop-types'
import React from 'react'
import TextField from '@mui/material/TextField'

/**
 * Client-side autocomplete for filter fields
 * Extracts unique values from the provided data instead of making API calls
 */
export default function GridToolbarClientAutocomplete(props) {
  const [open, setOpen] = React.useState(false)

  // Extract unique values from the data for this field
  const options = React.useMemo(() => {
    if (!props.data || !Array.isArray(props.data)) {
      return []
    }

    const uniqueValues = new Set()
    props.data.forEach((row) => {
      let value = row[props.field]

      // If valueGetter is provided, use it to get the display value
      if (props.valueGetter) {
        value = props.valueGetter({ row, value })
      }

      if (value !== null && value !== undefined && value !== '') {
        // Handle arrays (like variants in TriagedRegressionTestList)
        if (Array.isArray(value)) {
          value.forEach((v) => {
            const strValue = String(v)
            if (strValue !== '[object Object]') {
              uniqueValues.add(strValue)
            }
          })
        } else {
          const strValue = String(value)
          // Don't add objects that weren't converted properly
          if (strValue !== '[object Object]') {
            uniqueValues.add(strValue)
          }
        }
      }
    })

    // Convert to array and sort
    return Array.from(uniqueValues)
      .sort()
      .map((v) => ({ name: v }))
  }, [props.data, props.field, props.valueGetter])

  return (
    <Autocomplete
      freeSolo
      id={`autocomplete-${props.id}`}
      style={{ width: 220 }}
      open={open}
      onOpen={() => {
        setOpen(true)
      }}
      onClose={() => {
        setOpen(false)
      }}
      onChange={(e, v) => {
        if (typeof v === 'string') {
          props.onChange(v)
        } else if (v && v.name) {
          props.onChange(v.name)
        }
      }}
      onInputChange={(e, value) => {
        if (e && e.type === 'change') {
          props.onChange(value)
        }
      }}
      value={props.value || ''}
      inputValue={props.value || ''}
      isOptionEqualToValue={(option, value) =>
        option.name === (typeof value === 'string' ? value : value.name)
      }
      getOptionLabel={(option) =>
        typeof option === 'string' ? option : option.name
      }
      options={options}
      renderInput={(params) => (
        <TextField
          variant="standard"
          {...params}
          id={props.id}
          label={props.label}
          error={props.error}
        />
      )}
    />
  )
}

GridToolbarClientAutocomplete.propTypes = {
  id: PropTypes.string,
  error: PropTypes.bool,
  label: PropTypes.string,
  field: PropTypes.string,
  value: PropTypes.string,
  onChange: PropTypes.func,
  data: PropTypes.array.isRequired,
  valueGetter: PropTypes.func,
}
