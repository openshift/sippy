import { CrReleaseBox } from './CompReadyMainInputs'
import { FormControl, InputLabel, MenuItem, Select } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'

export default function ViewPicker(props) {
  if (!props.enabled) return null
  return (
    <CrReleaseBox>
      <FormControl variant="standard" sx={{ minWidth: 100 }}>
        <InputLabel> View </InputLabel>
        <Select
          variant="standard"
          value={props.varsContext.view}
          onChange={(e) => {
            console.log('changed view to: ' + e.target.value)
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
    </CrReleaseBox>
  )
}

ViewPicker.propTypes = {
  enabled: PropTypes.bool,
  varsContext: PropTypes.object,
}
