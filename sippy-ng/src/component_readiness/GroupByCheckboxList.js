import './CheckboxList.css'
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormGroup,
  Tooltip,
} from '@mui/material'
import { CompReadyVarsContext } from './CompReadyVars'
import { ExpandMore } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useContext, useState } from 'react'
import Typography from '@mui/material/Typography'

export default function GroupByCheckboxList(props) {
  const varsContext = useContext(CompReadyVarsContext)
  const useStyles = makeStyles((theme) => ({
    formControl: {
      margin: theme.spacing(1),
      minWidth: '20px',
    },
  }))

  const classes = useStyles()
  const handleChange = (event) => {
    const item = event.target.name
    const isChecked = event.target.checked
    if (isChecked) {
      props.setCheckedItems([...props.checkedItems, item])
    } else {
      props.setCheckedItems(
        props.checkedItems.filter((checkedItem) => checkedItem !== item)
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
            {props.displayList.map((item) => {
              let isCompareMode = varsContext.variantCrossCompare.includes(item)
              return (
                <Tooltip
                  key={item}
                  title={
                    isCompareMode
                      ? 'Column grouping is disabled for this variant while selected for cross-comparison'
                      : 'Separate columns by variants in group ' + item
                  }
                >
                  <FormControlLabel
                    key={item}
                    disabled={isCompareMode}
                    control={
                      <Checkbox
                        checked={props.checkedItems.includes(item)}
                        disabled={isCompareMode}
                        onChange={handleChange}
                        name={item}
                      />
                    }
                    label={item}
                  />
                </Tooltip>
              )
            })}
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
