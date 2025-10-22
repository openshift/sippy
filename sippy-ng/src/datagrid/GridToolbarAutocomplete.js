import { apiFetch, safeEncodeURIComponent } from '../helpers'
import Autocomplete from '@mui/lab/Autocomplete'
import CircularProgress from '@mui/material/CircularProgress'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'
import TextField from '@mui/material/TextField'

export default function GridToolbarAutocomplete(props) {
  const [open, setOpen] = React.useState(false)
  const [options, setOptions] = React.useState([])
  const [loading, setLoading] = React.useState(false)

  const fetchOptions = async (value) => {
    setLoading(true)
    let queryParams = []
    if (value !== '') {
      queryParams.push('search=' + safeEncodeURIComponent(value))
    }
    if (props.release !== '') {
      queryParams.push('release=' + safeEncodeURIComponent(props.release))
    }

    const response = await apiFetch(
      `/api/autocomplete/${props.field}?${queryParams.join('&')}`
    )

    const values = await response.json()
    let valueObj = []
    values.forEach((v) => valueObj.push({ name: v }))
    setOptions(valueObj)
    setLoading(false)
  }

  useEffect(() => {
    if (open) {
      fetchOptions(props.value)
    } else {
      setOptions([])
    }
  }, [open])

  // Based on https://stackoverflow.com/a/61973338/1683486
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
      defaultValue={{ name: props.value }}
      isOptionEqualToValue={(option, value) => option.name === value.name}
      getOptionLabel={(option) => option.name}
      options={options}
      loading={loading}
      renderInput={(params) => (
        <TextField
          variant="standard"
          {...params}
          id={props.id}
          label={props.label}
          error={props.error}
          onChange={(ev) => {
            // dont fire API if the user delete or not entered anything
            if (ev.target.value !== '' || ev.target.value !== null) {
              fetchOptions(ev.target.value)
            }
          }}
          InputProps={{
            ...params.InputProps,
            endAdornment: (
              <React.Fragment>
                {loading ? (
                  <CircularProgress color="inherit" size={20} />
                ) : null}
                {params.InputProps.endAdornment}
              </React.Fragment>
            ),
          }}
        />
      )}
    />
  )
}

GridToolbarAutocomplete.defaultProps = {
  release: '',
}

GridToolbarAutocomplete.propTypes = {
  id: PropTypes.string,
  error: PropTypes.string,
  label: PropTypes.string,
  release: PropTypes.string,
  field: PropTypes.string,
  value: PropTypes.string,
  onChange: PropTypes.func,
}
