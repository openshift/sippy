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
import { CompReadyVarsContext } from './CompReadyVars'
import { ExpandMore } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useContext, useState } from 'react'
import Typography from '@mui/material/Typography'

export default function IncludeVariantCheckBoxList(props) {
  const variantGroupName = props.variantGroupName
  const varsContext = useContext(CompReadyVarsContext)
  const [includeItems, setIncludeItems] = React.useState(
    variantGroupName in varsContext.includeVariantsCheckedItems
      ? varsContext.includeVariantsCheckedItems[variantGroupName]
      : []
  )
  const updateIncludeItems = (newItems) => {
    varsContext.replaceIncludeVariantsCheckedItems(variantGroupName, newItems)
    setIncludeItems(newItems)
  }

  const [compareItems, setCompareItems] = React.useState(
    variantGroupName in varsContext.compareVariantsCheckedItems
      ? varsContext.compareVariantsCheckedItems[variantGroupName]
      : []
  )
  const updateCompareItems = (newItems) => {
    varsContext.replaceCompareVariantsCheckedItems(variantGroupName, newItems)
    setCompareItems(newItems)
  }

  const handleVariantSelection = (
    event,
    checkedItems,
    setCheckedItems,
    updateCheckedItems
  ) => {
    const item = event.target.name
    const newItems = event.target.checked
      ? [...checkedItems, item]
      : checkedItems.filter((checkedItem) => checkedItem !== item)
    setCheckedItems(newItems)
    updateCheckedItems(newItems)
  }
  const handleIncludeVariantSelection = (event) => {
    handleVariantSelection(
      event,
      includeItems,
      setIncludeItems,
      updateIncludeItems
    )
  }
  const handleCompareVariantSelection = (event) => {
    handleVariantSelection(
      event,
      compareItems,
      setCompareItems,
      updateCompareItems
    )
  }

  const [isCompareMode, setIsCompareMode] = useState(
    varsContext.variantCrossCompare.includes(variantGroupName)
  )
  const handleToggleCompare = (event) => {
    varsContext.updateVariantCrossCompare(variantGroupName, !isCompareMode)
    setIsCompareMode(!isCompareMode)
  }

  const classes = makeStyles((theme) => ({
    formControl: {
      margin: theme.spacing(1),
      minWidth: '20px',
    },
    gridCenter: {
      marginTop: theme.spacing(1),
      textAlign: 'center',
    },
    gridRight: {
      justifyContent: 'flex-end',
      display: 'flex',
    },
  }))()

  let params = {
    variantGroupName,
    classes,
    displayList: varsContext.allJobVariants[variantGroupName],
    includeItems,
    handleIncludeVariantSelection,
    compareItems,
    handleCompareVariantSelection,
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
            Include {variantGroupName}
          </Typography>
        </AccordionSummary>
        <AccordionDetails>
          {isCompareMode
            ? CompareVariantGroup(params)
            : IncludeVariantGroup(params)}
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

function IncludeVariantGroup(params) {
  return (
    <FormGroup>
      {params.displayList.map((item) => (
        <FormControlLabel
          key={item}
          control={
            <Checkbox
              checked={params.includeItems.includes(item)}
              onChange={params.handleIncludeVariantSelection}
              name={item}
            />
          }
          label={item}
        />
      ))}
    </FormGroup>
  )
}

function CompareVariantGroup(params) {
  return (
    <FormGroup>
      <Grid container spacing={2}>
        <Grid item xs={6}>
          Basis
        </Grid>
        <Grid item xs={6} className={params.classes.gridRight}>
          Sample
        </Grid>
      </Grid>
      {params.displayList.map((item) => (
        <Grid container spacing={2} key={item}>
          <Grid item xs={2}>
            <Checkbox
              checked={params.includeItems.includes(item)}
              onChange={params.handleIncludeVariantSelection}
              name={item}
            />
          </Grid>
          <Grid item xs={8} className={params.classes.gridCenter}>
            {item}
          </Grid>
          <Grid item xs={2}>
            <Checkbox
              checked={params.compareItems.includes(item)}
              onChange={params.handleCompareVariantSelection}
              name={item}
            />
          </Grid>
        </Grid>
      ))}
    </FormGroup>
  )
}

IncludeVariantCheckBoxList.propTypes = {
  variantGroupName: PropTypes.string.isRequired,
}
