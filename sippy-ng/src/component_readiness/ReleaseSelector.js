import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns'
import { dateEndFormat, dateFormat, formatLongDate } from './CompReadyUtils'
import { DatePicker, LocalizationProvider } from '@mui/x-date-pickers'
import { Filter1, Filter2, Filter4, LocalShipping } from '@mui/icons-material'
import {
  FormControl,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
} from '@mui/material'
import { Fragment, useContext, useEffect } from 'react'
import { makeStyles } from '@mui/styles'
import { ReleasesContext } from '../App'
import PropTypes from 'prop-types'
import React from 'react'
import TextField from '@mui/material/TextField'

const useStyles = makeStyles((theme) => ({
  formControl: {
    margin: theme.spacing(1),
    minWidth: 80,
  },
  selectEmpty: {
    marginTop: theme.spacing(5),
  },
  label: {
    display: 'flex',
    whiteSpace: 'nowrap',
  },
}))

function ReleaseSelector(props) {
  const classes = useStyles()
  const releases = useContext(ReleasesContext)
  const [versions, setVersions] = React.useState({})
  const {
    label,
    setStartTime,
    startTime,
    setEndTime,
    endTime,
    version,
    onChange,
  } = props

  const days = 24 * 60 * 60 * 1000
  const oneWeekStart = new Date(new Date().getTime() - 7 * days)
  const twoWeeksStart = new Date(new Date().getTime() - 2 * 7 * days)
  const fourWeeksStart = new Date(new Date().getTime() - 4 * 7 * days)
  const defaultEndTime = new Date(new Date().getTime())

  const setGADate = () => {
    let start = new Date(versions[version])
    setStartTime(formatLongDate(start.setDate(start.getDate() - 28)))
    setEndTime(formatLongDate(versions[version]))
  }

  const set4Weeks = () => {
    setStartTime(fourWeeksStart)
    setEndTime(defaultEndTime)
  }

  const set2Weeks = () => {
    setStartTime(twoWeeksStart)
    setEndTime(defaultEndTime)
  }

  const set1Week = () => {
    setStartTime(oneWeekStart)
    setEndTime(defaultEndTime)
  }

  useEffect(() => {
    let tmpRelease = {}
    releases.releases
      .filter((aVersion) => {
        // We won't process Presubmits or 3.11
        return aVersion !== 'Presubmits' && aVersion != '3.11'
      })
      .forEach((r) => {
        tmpRelease[r] = releases.ga_dates[r]
      })
    setVersions(tmpRelease)
  }, [releases])

  // Ensure that versions has a list of versions before trying to display the Form
  if (Object.keys(versions).length === 0) {
    return <p>Loading Releases...</p>
  }

  return (
    <Fragment>
      <Grid container justifyContent="center" alignItems="center">
        <Grid item md={12}>
          <FormControl variant="standard" className={classes.formControl}>
            <InputLabel className={classes.label}>{label}</InputLabel>
            <Select variant="standard" value={version} onChange={onChange}>
              {Object.keys(versions).map((v) => (
                <MenuItem key={v} value={v}>
                  {v}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <LocalizationProvider dateAdapter={AdapterDateFns}>
            <DatePicker
              showTodayButton
              disableFuture
              label="From"
              format={dateFormat}
              ampm={false}
              value={startTime}
              onChange={(e) => {
                const formattedTime = formatLongDate(e, dateFormat)
                setStartTime(formattedTime)
              }}
              renderInput={(props) => <TextField {...props} />}
            />
            <DatePicker
              showTodayButton
              disableFuture
              label="To"
              format={dateEndFormat}
              ampm={false}
              value={endTime}
              onChange={(e) => {
                const formattedTime = formatLongDate(e, dateEndFormat)
                setEndTime(formattedTime)
              }}
              renderInput={(props) => <TextField {...props} />}
            />
          </LocalizationProvider>
        </Grid>
        <Grid item md={12} style={{ marginTop: 5 }}>
          <ToggleButtonGroup aria-label="release-dates">
            <Tooltip title="Last week">
              <ToggleButton
                variant="primary"
                onClick={set1Week}
                aria-label="filter-2"
                value=""
              >
                <Filter1 fontSize="small" />
              </ToggleButton>
            </Tooltip>
            <Tooltip title="Last 2 weeks">
              <ToggleButton onClick={set2Weeks} aria-label="filter-2" value="">
                <Filter2 fontSize="small" />
              </ToggleButton>
            </Tooltip>
            <Tooltip title="Last 4 weeks">
              <ToggleButton onClick={set4Weeks} aria-label="filter-4" value="">
                <Filter4 fontSize="small" />
              </ToggleButton>
            </Tooltip>
            <Tooltip title="4 weeks before GA">
              <ToggleButton
                onClick={setGADate}
                value=""
                aria-label="ga-date"
                fontSize="small"
                style={{
                  visibility:
                    versions[version] === undefined ||
                    versions[version] === null
                      ? 'hidden'
                      : 'visible',
                }}
              >
                <LocalShipping />
              </ToggleButton>
            </Tooltip>
          </ToggleButtonGroup>
        </Grid>
      </Grid>
    </Fragment>
  )
}

ReleaseSelector.propTypes = {
  startTime: PropTypes.string,
  setStartTime: PropTypes.func,
  endTime: PropTypes.string,
  setEndTime: PropTypes.func,
  label: PropTypes.string,
  version: PropTypes.string,
  onChange: PropTypes.func,
}

ReleaseSelector.defaultProps = {
  label: 'Version',
}

export default ReleaseSelector
