import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns'
import { dateEndFormat, dateFormat, formatLongDate } from './CompReadyUtils'
import { DatePicker, LocalizationProvider } from '@mui/x-date-pickers'
import { Filter1, Filter2, Filter4, LocalShipping } from '@mui/icons-material'
import {
  FormControl,
  FormHelperText,
  Grid,
  Input,
  InputLabel,
  MenuItem,
  Select,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
} from '@mui/material'
import { Fragment, useContext, useEffect, useState } from 'react'
import { makeStyles } from '@mui/styles'
import { ReleasesContext } from '../App'
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
  const releases = useContext(ReleasesContext)
  const [versions, setVersions] = useState({})
  const {
    label,
    setStartTime,
    startTime,
    setEndTime,
    endTime,
    version,
    onChange,
    pullRequestSupport,
    pullRequestOrg,
    setPullRequestOrg,
    pullRequestRepo,
    setPullRequestRepo,
    pullRequestNumber,
    setPullRequestNumber,
    payloadSupport,
    payloadTags,
    setPayloadTags,
  } = props

  const days = 24 * 60 * 60 * 1000
  const oneWeekStart = new Date(new Date().getTime() - 7 * days)
  const twoWeeksStart = new Date(new Date().getTime() - 2 * 7 * days)
  const fourWeeksStart = new Date(new Date().getTime() - 4 * 7 * days)
  const defaultEndTime = new Date(new Date().getTime())

  const [pullRequestURL, setPullRequestURL] = useState('')
  const [pullRequestURLError, setPullRequestURLError] = useState(false)

  const [payloadTagsError, setPayloadTagsError] = useState(false)

  const setGADate = () => {
    let start = new Date(versions[version])
    setStartTime(
      formatLongDate(start.setDate(start.getDate() - 27), dateFormat)
    )
    setEndTime(formatLongDate(versions[version], dateEndFormat))
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
        return releases.release_attrs[aVersion].capabilities.componentReadiness
      })
      .forEach((r) => {
        tmpRelease[r] = releases.ga_dates[r]
      })
    setVersions(tmpRelease)
  }, [releases])

  useEffect(() => {
    if (pullRequestOrg && pullRequestRepo && pullRequestNumber) {
      setPullRequestURL(
        `https://github.com/${pullRequestOrg}/${pullRequestRepo}/pull/${pullRequestNumber}`
      )
    }
  }, [pullRequestOrg, pullRequestRepo, pullRequestNumber])

  useEffect(() => {
    if (payloadTags && payloadTags.length !== 0) {
      setPayloadTags(payloadTags)
    }
  }, [payloadTags])

  const handlePullRequestURLChange = (e) => {
    const newURL = e.target.value
    setPullRequestURL(newURL)

    // Don't allow PRURL and payload tag at the same time
    if (payloadTags && payloadTags.length !== 0) {
      setPullRequestURLError(true)
      return
    }
    // Allow clearing the URL:
    if (newURL === '') {
      setPullRequestURLError(false)
      setPullRequestOrg('')
      setPullRequestRepo('')
      setPullRequestNumber('')
      return
    }

    const regex = /^https:\/\/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)$/
    const match = newURL.match(regex)
    if (match) {
      setPullRequestURLError(false)
      setPullRequestOrg(match[1])
      setPullRequestRepo(match[2])
      setPullRequestNumber(match[3])
    } else {
      setPullRequestURLError(true)
    }
  }

  const handlePayloadTagsChange = (e) => {
    const newTags = e.target.value

    // Don't allow PRURL and payload tag at the same time
    if (pullRequestURL !== '') {
      setPayloadTagsError(true)
      return
    }
    // Allow clearing the URL:
    if (newTags === '') {
      setPayloadTagsError(false)
      setPayloadTags([])
      return
    }

    // Match string like
    // 4.20.0-ec.3,
    // 4.19.0-rc.5,
    // 4.19.0,
    // 4.20.0-0.ci-2025-06-30-145044,
    // 4.20.0-0.konflux-nightly-2025-05-21-161707 and
    // 4.19.0-0.nightly-2025-06-30-135545
    const regex =
      /^\d+\.\d+\.\d+(?:-(?:ec\.\d+|rc\.\d+|\d+\.(?:ci|konflux-nightly|nightly)-\d{4}-\d{2}-\d{2}-\d{6}))?$/

    const tags = newTags.split(',')
    for (const tag of tags) {
      if (!tag.match(regex)) {
        setPayloadTagsError(true)
        setPayloadTags([])
        return
      }
    }
    setPayloadTagsError(false)
    setPayloadTags(tags)
  }

  // Ensure that versions has a list of versions before trying to display the Form
  if (Object.keys(versions).length === 0) {
    return <p>Loading Releases...</p>
  }

  // dateExtract takes a date from the DatePicker and extracts only the year, month, and day.
  // We can then use these 3 things to create a UTC time (regardless of the local browser's TZ).
  const dateExtractor = (descString, e) => {
    // Extract year, month, day as a string.
    console.log(`${descString} in: `, e)
    const year = e.getFullYear()
    const month = e.getMonth() + 1
    const day = e.getDate()
    const stringTime = `${year}-${month}-${day}`
    console.log(`${descString}: `, stringTime)
    return stringTime
  }

  return (
    <Fragment>
      <Grid container justifyContent="center" alignItems="center">
        <Grid item md={12}>
          <FormControl variant="standard" className={classes.formControl}>
            <Tooltip title={props.tooltip}>
              <InputLabel className={classes.label}>{label}</InputLabel>
            </Tooltip>
            <Select variant="standard" value={version} onChange={onChange}>
              {Object.keys(versions).map((v) => (
                <MenuItem key={v} value={v}>
                  {v}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <div>
            {pullRequestSupport && (
              <FormControl error={pullRequestURLError}>
                <InputLabel htmlFor="pullRequestURL">
                  Pull Request (optional)
                </InputLabel>
                <Input
                  id="pullRequestURL"
                  value={pullRequestURL}
                  onChange={handlePullRequestURLChange}
                />
                {pullRequestURLError &&
                  payloadTags &&
                  payloadTags.length !== 0 && (
                    <FormHelperText>
                      Cannot have payload tags and pull request URL at the same
                      time!
                    </FormHelperText>
                  )}
                {pullRequestURLError &&
                  (!payloadTags || payloadTags.length === 0) && (
                    <FormHelperText>Invalid Pull Request URL</FormHelperText>
                  )}
              </FormControl>
            )}
          </div>
          <div>
            {payloadSupport && (
              <FormControl error={payloadTagsError}>
                <InputLabel htmlFor="payloadTags">
                  Payload Tags (optional)
                </InputLabel>
                <Input
                  id="payloadTags"
                  value={payloadTags}
                  onChange={handlePayloadTagsChange}
                />
                {payloadTagsError && pullRequestURL !== '' && (
                  <FormHelperText>
                    Cannot have pull request URL and payload tag at the same
                    time!
                  </FormHelperText>
                )}
                {payloadTagsError && pullRequestURL === '' && (
                  <FormHelperText>
                    Valid format: comma separately tags like
                    4.19.0-0.ci-2025-05-17-032906,4.20.0-ec.3,4.19.0-rc.5,4.19.0
                  </FormHelperText>
                )}
              </FormControl>
            )}
          </div>

          <LocalizationProvider dateAdapter={AdapterDateFns}>
            <DatePicker
              showTodayButton
              disableFuture
              label="From"
              format={dateFormat}
              ampm={false}
              value={startTime}
              onChange={(e) => {
                const stringStartTime = dateExtractor('startTime', e)
                const formattedTime = formatLongDate(
                  stringStartTime,
                  dateFormat
                )
                setStartTime(formattedTime)
              }}
              renderInput={(props) => (
                <TextField variant="standard" {...props} />
              )}
            />
            <DatePicker
              showTodayButton
              disableFuture
              label="To"
              format={dateEndFormat}
              ampm={false}
              value={endTime}
              onChange={(e) => {
                const stringEndTime = dateExtractor('endTime', e)
                const formattedTime = formatLongDate(
                  stringEndTime,
                  dateEndFormat
                )
                setEndTime(formattedTime)
              }}
              renderInput={(props) => (
                <TextField variant="standard" {...props} />
              )}
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
  tooltip: PropTypes.string,
  version: PropTypes.string,
  onChange: PropTypes.func,
  pullRequestSupport: PropTypes.bool,
  pullRequestOrg: PropTypes.string,
  setPullRequestOrg: PropTypes.func,
  pullRequestRepo: PropTypes.string,
  setPullRequestRepo: PropTypes.func,
  pullRequestNumber: PropTypes.string,
  setPullRequestNumber: PropTypes.func,
  payloadSupport: PropTypes.bool,
  payloadTags: PropTypes.string,
  setPayloadTags: PropTypes.func,
}

ReleaseSelector.defaultProps = {
  label: 'Version',
  pullRequestSupport: false,
  payloadSupport: false,
}

export default ReleaseSelector
