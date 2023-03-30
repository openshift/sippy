import './CheckboxList.css'
import {
  Checkbox,
  FormControl,
  FormControlLabel,
  FormGroup,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Typography from '@material-ui/core/Typography'

export default function CheckBoxList(props) {
  const useStyles = makeStyles((theme) => ({
    formControl: {
      margin: theme.spacing(1),
      minWidth: '20px',
    },
    selectEmpty: {
      marginTop: theme.spacing(2),
    },
  }))

  const classes = useStyles()
  const checkedItems = props.checkedItems
  const setCheckedItems = props.setCheckedItems
  const handleChange = (event) => {
    const item = event.target.name
    const isChecked = event.target.checked
    if (isChecked) {
      setCheckedItems([...checkedItems, item])
    } else {
      setCheckedItems(
        checkedItems.filter((checkedItem) => checkedItem !== item)
      )
    }
  }

  return (
    <FormControl className={classes.formControl} component="fieldset">
      <Typography className="checkboxlist-label">{props.headerName}</Typography>
      <FormGroup>
        {props.displayList.map((item) => (
          <FormControlLabel
            key={item}
            control={
              <Checkbox
                checked={checkedItems.includes(item)}
                onChange={handleChange}
                name={item}
              />
            }
            label={item}
          />
        ))}
      </FormGroup>
    </FormControl>
  )
}

CheckBoxList.propTypes = {
  headerName: PropTypes.string,
  displayList: PropTypes.array,
  checkedItems: PropTypes.array,
  setCheckedItems: PropTypes.func,
}
