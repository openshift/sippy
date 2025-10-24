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
      const value = row[props.field]
      if (value !== null && value !== undefined && value !== '') {
        // Handle arrays (like variants)
        if (Array.isArray(value)) {
          value.forEach((v) => uniqueValues.add(String(v)))
        } else {
          uniqueValues.add(String(value))
        }
      }
    })

    // Convert to array and sort
    return Array.from(uniqueValues)
      .sort()
      .map((v) => ({ name: v }))
  }, [props.data, props.field])

  return (
    <Autocomplete
      disableClearable
      id={`autocomplete-${props.id}`}
      style={{ width: 220 }}
      open={open}
      onOpen={() => {
        setOpen(true)
      }}
      onClose={() => {
        setOpen(false)
      }}
      onChange={(e, v) => v && props.onChange(v.name)}
      value={props.value ? { name: props.value } : null}
      isOptionEqualToValue={(option, value) => option.name === value.name}
      getOptionLabel={(option) => option.name}
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
}
