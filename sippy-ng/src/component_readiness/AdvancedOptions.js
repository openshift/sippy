import './CheckboxList.css'
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  FormControl,
  FormGroup,
  Tooltip,
} from '@mui/material'
import { ExpandMore } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'

import PropTypes from 'prop-types'
import React, { useState } from 'react'
import Slider from '@mui/material/Slider'
import Switch from '@mui/material/Switch'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

export default function AdvancedOptions(props) {
  const {
    headerName,
    confidence,
    pity,
    minFail,
    passRateNewTests,
    passRateAllTests,
    ignoreMissing,
    ignoreDisruption,
    flakeAsFailure,
    includeMultiReleaseAnalysis,
    setConfidence,
    setPity,
    setMinFail,
    setPassRateNewTests,
    setPassRateAllTests,
    setIgnoreMissing,
    setIgnoreDisruption,
    setFlakeAsFailure,
    setIncludeMultiReleaseAnalysis,
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
  const handleChangePassRateNewTests = (event, newValue) => {
    let value = parseInt(event.target.value, 10)

    if (value < 0) {
      value = 0
    } else if (value > 100) {
      value = 100
    }
    setPassRateNewTests(value)
  }
  const handleChangePassRateAllTests = (event, newValue) => {
    let value = parseInt(event.target.value, 10)

    if (value < 0) {
      value = 0
    } else if (value > 100) {
      value = 100
    }
    setPassRateAllTests(value)
  }
  const handleChangeIgnoreMissing = (event, newValue) => {
    setIgnoreMissing(newValue)
  }
  const handleChangeIgnoreDisruption = (event, newValue) => {
    setIgnoreDisruption(newValue)
  }
  const handleChangeFlakeAsFailure = (event, newValue) => {
    setFlakeAsFailure(newValue)
  }

  const handleChangeIncludeMultiReleaseAnalysis = (event, newValue) => {
    setIncludeMultiReleaseAnalysis(newValue)
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
          <FormGroup>
            <p>Confidence: {confidence}</p>
            <Slider
              value={confidence}
              onChange={handleChangeConfidence}
              aria-labelledby="my-slider"
              min={80}
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
            <p>Require new tests pass rate:</p>
            <TextField
              value={passRateNewTests}
              type="number"
              size="small"
              onChange={handleChangePassRateNewTests}
              inputProps={{ min: 0, max: 100 }}
            />

            <p>Require all tests pass rate:</p>
            <TextField
              value={passRateAllTests}
              type="number"
              size="small"
              onChange={handleChangePassRateAllTests}
              inputProps={{ min: 0, max: 100 }}
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
            <p>Flake As: {flakeAsFailure ? 'failure' : 'success'}</p>
            <Switch
              checked={flakeAsFailure}
              onChange={handleChangeFlakeAsFailure}
              name="flakeAsFailure"
              color="primary"
            />
            <Tooltip title="Enable analysis across multiple prior releases">
              <p>
                Historical release analysis:{' '}
                {includeMultiReleaseAnalysis ? 'include' : 'exclude'}
              </p>
              <Switch
                checked={includeMultiReleaseAnalysis}
                onChange={handleChangeIncludeMultiReleaseAnalysis}
                name="includeMultiReleaseAnalysis"
                color="primary"
              />
            </Tooltip>
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
  passRateNewTests: PropTypes.number.isRequired,
  passRateAllTests: PropTypes.number.isRequired,
  ignoreMissing: PropTypes.bool.isRequired,
  ignoreDisruption: PropTypes.bool.isRequired,
  flakeAsFailure: PropTypes.bool.isRequired,
  includeMultiReleaseAnalysis: PropTypes.bool.isRequired,
  setConfidence: PropTypes.func.isRequired,
  setPity: PropTypes.func.isRequired,
  setMinFail: PropTypes.func.isRequired,
  setPassRateNewTests: PropTypes.func.isRequired,
  setPassRateAllTests: PropTypes.func.isRequired,
  setIgnoreMissing: PropTypes.func.isRequired,
  setIgnoreDisruption: PropTypes.func.isRequired,
  setFlakeAsFailure: PropTypes.func.isRequired,
  setIncludeMultiReleaseAnalysis: PropTypes.func.isRequired,
}
