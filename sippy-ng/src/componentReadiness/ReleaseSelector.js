import { FormControl, InputLabel, MenuItem, Select } from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles((theme) => ({
  formControl: {
    margin: theme.spacing(1),
    minWidth: 80,
  },
  selectEmpty: {
    marginTop: theme.spacing(5),
  },
}))

function ReleaseSelector(props) {
  const classes = useStyles()
  const { label, version, versions, onChange } = props

  const handleChange = (event) => {
    onChange(event.target.value)
  }

  return (
    <FormControl className={classes.formControl}>
      <InputLabel>{label}</InputLabel>
      <Select value={version} onChange={handleChange}>
        {versions.map((v) => (
          <MenuItem key={v} value={v}>
            {v}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  )
}

ReleaseSelector.propTypes = {
  label: PropTypes.string,
  version: PropTypes.string,
  versions: PropTypes.array,
  onChange: PropTypes.func,
}

ReleaseSelector.defaultProps = {
  label: 'Version',
  versions: ['4.10', '4.11', '4.12', '4.13', '4.14'],
}

export default ReleaseSelector
