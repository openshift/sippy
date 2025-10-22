import { Box, Chip, Tooltip, Typography, useTheme } from '@mui/material'
import {
  CheckCircle,
  Error as ErrorIcon,
  Help,
  Warning,
} from '@mui/icons-material'
import {
  apiFetch,
  getReportStartDate,
  relativeTime,
  safeEncodeURIComponent,
} from '../helpers'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { ReportEndContext } from '../App'
import Alert from '@mui/material/Alert'
import Grid from '@mui/material/Grid'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

const useStyles = makeStyles((theme) => ({
  payloadItem: {
    textDecoration: 'none',
    display: 'flex',
    alignItems: 'center',
    padding: theme.spacing(1.5),
    borderRadius: theme.spacing(1),
    border: `2px solid ${theme.palette.divider}`,
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
}))

function ReleasePayloadAcceptance(props) {
  const theme = useTheme()
  const classes = useStyles()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])
  const startDate = getReportStartDate(React.useContext(ReportEndContext))

  const fetchData = () => {
    apiFetch(
      '/api/releases/health?release=' + safeEncodeURIComponent(props.release)
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setRows(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve tags ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (fetchError !== '') {
    return (
      <Grid item xs={12}>
        <Alert severity="error">{fetchError}</Alert>
      </Grid>
    )
  }

  if (isLoaded === false) {
    return (
      <Grid item xs={12}>
        <Box sx={{ p: 2, textAlign: 'center', color: 'text.secondary' }}>
          <Typography>Loading payload acceptance data...</Typography>
        </Box>
      </Grid>
    )
  }

  let items = []
  rows.forEach((row) => {
    let icon = <Help />
    let text = 'Unknown'
    let tooltip = 'No information is available.'
    let chipColor = 'default'
    let when = startDate.getTime() - new Date(row.release_time).getTime()

    if (row.release_time && row.release_time != '') {
      tooltip = `The last ${row.count} releases were ${row.last_phase}`
      text = relativeTime(new Date(row.release_time), startDate)

      if (row.last_phase === 'Accepted' && when <= 24 * 60 * 60 * 1000) {
        // If we had an accepted release in the last 24 hours, we're green
        icon = <CheckCircle sx={{ color: theme.palette.success.main }} />
        chipColor = 'success'
      } else if (row.last_phase === 'Rejected') {
        // If the last payload was rejected, we are red.
        icon = <ErrorIcon sx={{ color: theme.palette.error.main }} />
        chipColor = 'error'
      } else {
        // Otherwise we are yellow -- e.g., last release payload was accepted
        // but it's been several days.
        icon = <Warning sx={{ color: theme.palette.warning.main }} />
        chipColor = 'warning'
      }
    }

    items.push(
      <Grid
        item
        xs={12}
        sm={6}
        md={4}
        lg={3}
        key={`release-${props.release}-${row.architecture}-${row.stream}`}
      >
        <Tooltip title={tooltip}>
          <Box
            component={Link}
            to={`/release/${props.release}/streams/${row.architecture}/${row.stream}/overview`}
            className={classes.payloadItem}
          >
            <Box sx={{ mr: 1.5 }}>{icon}</Box>
            <Box sx={{ flex: 1 }}>
              <Typography variant="subtitle2">{row.architecture}</Typography>
              <Typography variant="caption" color="text.secondary">
                {row.stream}
              </Typography>
            </Box>
            <Box sx={{ textAlign: 'right' }}>
              <Chip label={row.last_phase} color={chipColor} size="small" />
              <Typography
                variant="caption"
                display="block"
                color="text.secondary"
              >
                {text}
              </Typography>
            </Box>
          </Box>
        </Tooltip>
      </Grid>
    )
  })

  return <>{items}</>
}

ReleasePayloadAcceptance.defaultProps = {}

ReleasePayloadAcceptance.propTypes = {
  release: PropTypes.string.isRequired,
}

export default ReleasePayloadAcceptance
