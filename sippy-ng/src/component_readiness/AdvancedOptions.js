import './CheckboxList.css'
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  FormControl,
  FormControlLabel,
  FormGroup,
} from '@material-ui/core'
import { ExpandMore } from '@material-ui/icons'
import { makeStyles } from '@material-ui/core/styles'

import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Slider from '@material-ui/core/Slider'
import Typography from '@material-ui/core/Typography'

export default function AdvancedOptions(props) {
  const { displayList, checkedItems, setCheckedItems } = props
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
  }))

  const classes = useStyles()

  const [confidence, setConfidence] = useState(95)
  const [pity, setPity] = useState(5)
  const [minFail, setMinFail] = useState(3)
  const [ignoreMissing, setIgnoreMissing] = useState(0)
  const [ignoreDisruption, setIgnoreDisrupiton] = useState(0)

  const handleChangeConfidence = (event, newValue) => {
    setConfidence(newValue)
  }
  const handleChangePity = (event, newValue) => {
    setPity(newValue)
  }
  const handleChangeMinFail = (event, newValue) => {
    setMinFail(newValue)
  }
  const handleChangeIgnoreMissing = (event, newValue) => {
    setIgnoreMissing(newValue)
  }
  const handleChangeIgnoreDisruption = (event, newValue) => {
    setIgnoreDisrupiton(newValue)
  }

  return (
    <FormControl className={classes.formControl} component="fieldset">
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
            <p>Confidence: {confidence}</p>
            <Slider
              value={confidence}
              onChange={handleChangeConfidence}
              aria-labelledby="my-slider"
              min={1}
              max={100}
            />
            <p>Pity: {pity}</p>
            <Slider
              value={pity}
              onChange={handleChangePity}
              aria-labelledby="my-slider"
              min={1}
              max={100}
            />
            <p>MinFail: {minFail}</p>
            <Slider
              value={minFail}
              onChange={handleChangeMinFail}
              aria-labelledby="my-slider"
              min={1}
              max={3}
            />
            <p>Missing: {ignoreMissing ? 'ignore' : 'keep'}</p>
            <Slider
              value={ignoreMissing}
              onChange={handleChangeIgnoreMissing}
              aria-labelledby="my-slider"
              min={0}
              max={1}
            />
            <p>Disruption: {ignoreDisruption ? 'ignore' : 'keep'}</p>
            <Slider
              value={ignoreDisruption}
              onChange={handleChangeIgnoreDisruption}
              aria-labelledby="my-slider"
              min={0}
              max={1}
            />
          </FormGroup>
        </AccordionDetails>
      </Accordion>
    </FormControl>
  )
}

AdvancedOptions.propTypes = {
  headerName: PropTypes.string,
  displayList: PropTypes.array,
  checkedItems: PropTypes.array,
  setCheckedItems: PropTypes.func,
}
