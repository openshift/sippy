import './CheckboxList.css'
import { styled } from '@mui/material/styles';
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
import { makeStyles } from '@mui/material/styles'
import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Typography from '@mui/material/Typography'

const PREFIX = 'CheckboxList';

const classes = {
  formControl: `${PREFIX}-formControl`,
  selectEmpty: `${PREFIX}-selectEmpty`,
  headerName: `${PREFIX}-headerName`,
  summary: `${PREFIX}-summary`
};

const StyledFormControl = styled(FormControl)((
  {
    theme
  }
) => ({
  [`&.${classes.formControl}`]: {
    margin: theme.spacing(1),
    minWidth: '20px',
  },

  [`& .${classes.selectEmpty}`]: {
    marginTop: theme.spacing(2),
  },

  [`& .${classes.headerName}`]: {
    width: '220px',
    padding: '0px',
    margin: '0px',
  },

  [`& .${classes.summary}`]: {
    backgroundColor: 'rgb(0, 153, 255)',
    margin: '0px !important',
    padding: '0px',
  }
}));

export default function CheckBoxList(props) {

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
    <StyledFormControl
      variant="standard"
      className={classes.formControl}
      component="fieldset"
    >
      <Accordion className={classes.headerName}>
        <AccordionSummary
          className={classes.summary}
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
    </StyledFormControl>
  );
}

CheckBoxList.propTypes = {
  headerName: PropTypes.string,
  displayList: PropTypes.array,
  checkedItems: PropTypes.array,
  setCheckedItems: PropTypes.func,
}
