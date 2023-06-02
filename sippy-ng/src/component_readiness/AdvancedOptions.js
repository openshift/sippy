import './CheckboxList.css'
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  FormControl,
  FormGroup,
} from '@material-ui/core'
import { ExpandMore } from '@material-ui/icons'
import { makeStyles } from '@material-ui/core/styles'

import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Slider from '@material-ui/core/Slider'
import Switch from '@material-ui/core/Switch'
import Typography from '@material-ui/core/Typography'

export default function AdvancedOptions(props) {
  const {
    headerName,
    confidence,
    pity,
    minFail,
    ignoreMissing,
    ignoreDisruption,
    setConfidence,
    setPity,
    setMinFail,
    setIgnoreMissing,
    setIgnoreDisruption,
  } = props

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
    setIgnoreDisruption(newValue)
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
              min={90}
              max={100}
            />
            <p>Pity: {pity}</p>
            <Slider
              value={pity}
              onChange={handleChangePity}
              aria-labelledby="my-slider"
              min={0}
              max={10}
            />
            <p>MinFail: {minFail}</p>
            <Slider
              value={minFail}
              onChange={handleChangeMinFail}
              aria-labelledby="my-slider"
              min={0}
              max={20}
            />
            <p>Missing: {ignoreMissing ? 'ignore' : 'keep'}</p>
            <Switch
              checked={ignoreMissing}
              onChange={handleChangeIgnoreMissing}
              name="ignoreMissing"
              color="primary"
            />
            <p>Disruption: {ignoreDisruption ? 'ignore' : 'keep'}</p>
            <Switch
              checked={ignoreDisruption}
              onChange={handleChangeIgnoreDisruption}
              name="ignoreDisruption"
              color="primary"
            />
          </FormGroup>
        </AccordionDetails>
      </Accordion>
    </FormControl>
  )
}

AdvancedOptions.propTypes = {
  headerName: PropTypes.string.isRequired,
  confidence: PropTypes.number.isRequired,
  pity: PropTypes.number.isRequired,
  minFail: PropTypes.number.isRequired,
  ignoreMissing: PropTypes.bool.isRequired,
  ignoreDisruption: PropTypes.bool.isRequired,
  setConfidence: PropTypes.func.isRequired,
  setPity: PropTypes.func.isRequired,
  setMinFail: PropTypes.func.isRequired,
  setIgnoreMissing: PropTypes.func.isRequired,
  setIgnoreDisruption: PropTypes.func.isRequired,
}
