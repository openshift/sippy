import { safeEncodeURIComponent } from '../helpers'
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

    const response = await fetch(
      process.env.REACT_APP_API_URL +
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
