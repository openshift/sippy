import { FormControl, InputLabel, MenuItem, Select } from '@mui/material'
import { useStyles } from './CompReadyMainInputs'
import { useTheme } from '@mui/styles'
import PropTypes from 'prop-types'
import React from 'react'

// eslint-disable-next-line react/prop-types
export default function ViewPicker(props) {
  const theme = useTheme()
  const classes = useStyles(theme)
  if (!props.enabled) return null
  return (
    <div className={classes.crRelease}>
      <FormControl variant="standard">
        <InputLabel> View </InputLabel>
        <Select
          variant="standard"
          value={props.varsContext.view}
          onChange={(e) => {
            console.log('changed view to: ' + e.target.value)
            props.varsContext.setView(e.target.value)
            props.varsContext.views.forEach(function (item) {
              if (item.name === e.target.value) {
                // Update all inputs to match the values of the selected view, allowing the user
                // to customize easily:
                props.varsContext.syncView(item)
              }
            })
          }}
        >
          {props.varsContext.views.map((v, index) => (
            <MenuItem key={index} value={v.name}>
              {v.name}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
    </div>
  )
}

ViewPicker.propTypes = {
  enabled: PropTypes.bool,
  varsContext: PropTypes.Object,
}
