import {
  Box,
  Card,
  CardContent,
  Grid,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  CheckCircle,
  Error as ErrorIcon,
  Help,
  Warning,
} from '@mui/icons-material'
import { createTheme } from '@mui/material/styles'
import {
  getReportStartDate,
  relativeTime,
  safeEncodeURIComponent,
} from '../helpers'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { ReportEndContext } from '../App'
import Alert from '@mui/lab/Alert'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    releasePayloadOK: {
      backgroundColor: theme.palette.success.light,
    },
    releasePayloadProblem: {
      backgroundColor: theme.palette.error.light,
    },
  }),
  { defaultTheme }
)

function ReleasePayloadAcceptance(props) {
  const classes = useStyles()
  const theme = defaultTheme

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])
  const startDate = getReportStartDate(React.useContext(ReportEndContext))

  const fetchData = () => {
    fetch(
      process.env.REACT_APP_API_URL +
        '/api/releases/health?release=' +
        safeEncodeURIComponent(props.release)
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
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading...</p>
  }

  let cards = []
  rows.forEach((row) => {
    let bgColor = theme.palette.grey.A100
    let icon = <Help />
    let text = 'Unknown'
    let tooltip = 'No information is available.'
    let when = startDate.getTime() - new Date(row.release_time).getTime()
    if (row.release_time && row.release_time != '') {
      tooltip = `The last ${row.count} releases were ${row.last_phase}`
      text = relativeTime(new Date(row.release_time), startDate)

      if (row.last_phase === 'Accepted' && when <= 24 * 60 * 60 * 1000) {
        // If we had an accepted release in the last 24 hours, we're green
        bgColor = theme.palette.success.light
        icon = <CheckCircle style={{ fill: 'green' }} />
      } else if (row.last_phase === 'Rejected') {
        // If the last payload was rejected, we are red.
        bgColor = theme.palette.error.light
        icon = <ErrorIcon style={{ fill: 'maroon' }} />
      } else {
        // Otherwise we are yellow -- e.g., last release payload was accepted
        // but it's been several days.
        bgColor = theme.palette.warning.light
        icon = <Warning style={{ fill: 'goldenrod' }} />
      }
    }

    cards.push(
      <Box
        component={Link}
        to={`/release/${props.release}/streams/${row.architecture}/${row.stream}/overview`}
      >
        <Tooltip title={tooltip}>
          <Card
            elevation={5}
            style={{ backgroundColor: bgColor, margin: 20, width: 200 }}
          >
            <CardContent
              className={`${classes.cardContent}`}
              style={{ textAlign: 'center' }}
            >
              <Typography variant="h6">
                {row.architecture} {row.stream}
              </Typography>
              <Grid
                container
                direction="row"
                alignItems="center"
                style={{ margin: 20, textAlign: 'center' }}
              >
                {icon}&nbsp;{text}
              </Grid>
            </CardContent>
          </Card>
        </Tooltip>
      </Box>
    )
  })

  return cards
}

ReleasePayloadAcceptance.defaultProps = {}

ReleasePayloadAcceptance.propTypes = {
  release: PropTypes.string.isRequired,
}

export default ReleasePayloadAcceptance
