import './CheckboxList.css'
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormGroup,
  Grid,
  Tooltip,
} from '@mui/material'
import { ExpandMore } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Typography from '@mui/material/Typography'

export default function CheckBoxList(props) {
  const useStyles = makeStyles((theme) => ({
    formControl: {
      margin: theme.spacing(1),
      minWidth: '20px',
    },
    selectEmpty: {
      marginTop: theme.spacing(2),
    },
    headerName: {
      width: '220px',
      padding: '0px',
      margin: '0px',
    },
    summary: {
      backgroundColor: 'rgb(0, 153, 255)',
      margin: '0px !important',
      padding: '0px',
    },
    gridCenter: {
      marginTop: theme.spacing(1),
      textAlign: 'center',
    },
    gridRight: {
      justifyContent: 'flex-end',
      display: 'flex',
    },
  }))

  const classes = useStyles()
  const [checkedItems, setCheckedItems] = useState(props.checkedItems)
  const [isCompareMode, setIsCompareMode] = useState(false)
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
  const handleToggleCompare = () => {
    console.log('Toggle Compare')
    setIsCompareMode(!isCompareMode)
  }

  return (
    <FormControl
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
          {VariantGroup(
            props,
            classes,
            isCompareMode,
            checkedItems,
            handleChange
          )}
          <Tooltip
            title={'Compare with different variants for sample and basis'}
          >
            <Button
              variant="contained"
              color="primary"
              className={classes.control}
              onClick={handleToggleCompare}
            >
              {isCompareMode ? 'Cancel Compare' : 'Compare'}
            </Button>
          </Tooltip>
        </AccordionDetails>
      </Accordion>
    </FormControl>
  )
}

function VariantGroup(
  props,
  classes,
  isCompareMode,
  checkedItems,
  handleChange
) {
  if (isCompareMode) {
    return (
      <FormGroup>
        <Grid container spacing={2}>
          <Grid item xs={6}>
            Basis
          </Grid>
          <Grid item xs={6} className={classes.gridRight}>
            Sample
          </Grid>
        </Grid>
        {props.displayList.map((item) => (
          <Grid container spacing={2} key={item}>
            <Grid item xs={2}>
              <Checkbox
                checked={checkedItems.includes(item)}
                onChange={handleChange}
                name={item}
              />
            </Grid>
            <Grid item xs={8} className={classes.gridCenter}>
              {item}
            </Grid>
            <Grid item xs={2}>
              <Checkbox
                checked={checkedItems.includes('sample_' + item)}
                onChange={handleChange}
                name={'sample_' + item}
              />
            </Grid>
          </Grid>
        ))}
      </FormGroup>
    )
  }
  return (
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
  )
}

CheckBoxList.propTypes = VariantGroup.propTypes = {
  headerName: PropTypes.string,
  displayList: PropTypes.array,
  checkedItems: PropTypes.array,
  setCheckedItems: PropTypes.func,
}
