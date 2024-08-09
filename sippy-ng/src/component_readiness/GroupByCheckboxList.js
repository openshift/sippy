import './CheckboxList.css'
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormGroup,
} from '@mui/material'
import { ExpandMore } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Typography from '@mui/material/Typography'

export default function GroupByCheckboxList(props) {
  const useStyles = makeStyles((theme) => ({
    formControl: {
      margin: theme.spacing(1),
      minWidth: '20px',
    },
  }))

  const classes = useStyles()
  const [checkedItems, setCheckedItems] = useState(props.checkedItems)
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
    <FormControl
      variant="standard"
      className={classes.formControl}
      component="fieldset"
    >
      <Accordion className="checkboxlist-headerName">
        <AccordionSummary
          className="checkboxlist-summary"
          expandIcon={<ExpandMore />}
        >
          <Typography className="checkboxlist-label">
            {props.headerName}
          </Typography>
        </AccordionSummary>
        <AccordionDetails>
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
        </AccordionDetails>
      </Accordion>
    </FormControl>
  )
}

GroupByCheckboxList.propTypes = {
  headerName: PropTypes.string,
  displayList: PropTypes.array,
  checkedItems: PropTypes.array,
  setCheckedItems: PropTypes.func,
}
