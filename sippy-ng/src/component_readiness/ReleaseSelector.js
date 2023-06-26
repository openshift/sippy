import { dateEndFormat, dateFormat, formatLongDate } from './CompReadyUtils'
import { DatePicker, MuiPickersUtilsProvider } from '@material-ui/pickers'
import { Filter2, Filter4, LocalShipping } from '@material-ui/icons'
import {
  FormControl,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  Tooltip,
} from '@material-ui/core'
import { Fragment, useEffect } from 'react'
import { GridToolbarFilterDateUtils } from '../datagrid/GridToolbarFilterDateUtils'
import { makeStyles } from '@material-ui/core/styles'
import { ToggleButton, ToggleButtonGroup } from '@material-ui/lab'
import PropTypes from 'prop-types'
import React from 'react'

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
  const [versions, setVersions] = React.useState({})
  const {
    label,
    version,
    onChange,
    startTime,
    setStartTime,
    endTime,
    setEndTime,
  } = props

  const days = 24 * 60 * 60 * 1000
  const twoWeeksStart = new Date(new Date().getTime() - 2 * 7 * days)
  const fourWeeksStart = new Date(new Date().getTime() - 4 * 7 * days)
  const defaultEndTime = new Date(new Date().getTime())

  const fetchData = () => {
    fetch(process.env.REACT_APP_API_URL + '/api/releases')
      .then((response) => response.json())
      .then((data) => {
        let tmpRelease = {}
        data.releases
          .filter((aVersion) => {
            // We won't process Presubmits or 3.11
            return aVersion !== 'Presubmits' && aVersion != '3.11'
          })
          .forEach((r) => {
            tmpRelease[r] = data.ga_dates[r]
          })
        setVersions(tmpRelease)
      })
      .catch((error) => console.error(error))
  }

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

  useEffect(() => {
    fetchData()
  }, [])

  const handleChange = (event) => {
    onChange(event.target.value)
  }

  // Ensure that versions has a list of versions before trying to display the Form
  if (Object.keys(versions).length === 0) {
    return <p>Loading Releases...</p>
  }

  return (
    <Fragment>
      <Grid container justifyContent="center" alignItems="center">
        <Grid item md={12}>
          <FormControl className={classes.formControl}>
            <InputLabel className={classes.label}>{label}</InputLabel>
            <Select value={version} onChange={handleChange}>
              {Object.keys(versions).map((v) => (
                <MenuItem key={v} value={v}>
                  {v}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <MuiPickersUtilsProvider utils={GridToolbarFilterDateUtils}>
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
            />
          </MuiPickersUtilsProvider>
          <MuiPickersUtilsProvider utils={GridToolbarFilterDateUtils}>
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
            />
          </MuiPickersUtilsProvider>
        </Grid>
        <Grid item sm={12} md={6} style={{ marginTop: 5 }}>
          <ToggleButtonGroup size="small" ria-label="release-dates">
            <Tooltip title="Last 2 weeks">
              <ToggleButton onClick={set2Weeks} aria-label="filter-2">
                <Filter2 fontSize="small" />
              </ToggleButton>
            </Tooltip>
            <Tooltip title="Last 4 weeks">
              <ToggleButton onClick={set4Weeks} aria-label="filter-4">
                <Filter4 fontSize="small" />
              </ToggleButton>
            </Tooltip>
            <Tooltip title="4 weeks before GA">
              <ToggleButton
                onClick={setGADate}
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
  versions: PropTypes.array,
  onChange: PropTypes.func,
}

ReleaseSelector.defaultProps = {
  label: 'Version',
}

export default ReleaseSelector
