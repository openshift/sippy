import './CheckboxList.css'
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Autocomplete,
  Chip,
  FormControl,
  TextField,
  Tooltip,
} from '@mui/material'
import { CompReadyVarsContext } from './CompReadyVars'
import { ExpandMore } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import { TestCapabilitiesContext } from './ComponentReadiness'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import Typography from '@mui/material/Typography'

export default function SidebarTestFilters(props) {
  if (!props.controlsOpts?.filterByCapabilities) {
    // if we have no filters to show, omit the whole component; for now we only have capabilities as a filter
    return <Fragment />
  }
  const varsContext = useContext(CompReadyVarsContext)
  const testCapabilities = useContext(TestCapabilitiesContext)
  const useStyles = makeStyles((theme) => ({
    formControl: {
      margin: theme.spacing(1),
    },
    autocomplete: {
      width: '100%',
      // Override Material-UI Autocomplete padding that would make space for the dropdown triangle we disabled
      '& .MuiAutocomplete-inputRoot': {
        paddingRight: '9px !important',
      },
    },
    chip: {
      maxWidth: 'none',
      flex: '1 1 auto',
    },
  }))

  const classes = useStyles()

  const handleChange = (event, newValue) => {
    varsContext.setTestCapabilities(newValue || [])
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
        <AccordionDetails
          style={{
            paddingTop: '16px',
            paddingLeft: '8px',
            paddingRight: '8px',
          }}
        >
          {props.controlsOpts?.filterByCapabilities && (
            <Autocomplete
              multiple
              disableClearable
              className={classes.autocomplete}
              options={testCapabilities || []}
              value={varsContext.testCapabilities || []}
              onChange={handleChange}
              renderOption={(props, option) => (
                <li
                  {...props}
                  style={{ whiteSpace: 'normal', wordBreak: 'break-word' }}
                >
                  {option}
                </li>
              )}
              renderTags={(value, getTagProps) =>
                value.map((option, index) => (
                  <Tooltip title={option} key={option} placement="top">
                    <Chip
                      variant="outlined"
                      label={option}
                      className={classes.chip}
                      {...getTagProps({ index })}
                    />
                  </Tooltip>
                ))
              }
              renderInput={(params) => (
                <TextField
                  {...params}
                  variant="outlined"
                  label="Capabilities"
                  placeholder="Select capabilities"
                  InputProps={{
                    ...params.InputProps,
                    endAdornment: null, // Remove the dropdown triangle that takes up precious space with no real benefit
                  }}
                />
              )}
            />
          )}
        </AccordionDetails>
      </Accordion>
    </FormControl>
  )
}

SidebarTestFilters.propTypes = {
  controlsOpts: PropTypes.object,
  headerName: PropTypes.string,
}
