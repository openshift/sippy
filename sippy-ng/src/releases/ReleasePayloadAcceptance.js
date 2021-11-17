import { Box, Card, CardContent, Grid, Typography } from '@material-ui/core'
import { CheckCircle, Error, Help } from '@material-ui/icons'
import { createTheme, makeStyles } from '@material-ui/core/styles'
import { filterFor, relativeTime } from '../helpers'
import { Link } from 'react-router-dom'
import Alert from '@material-ui/lab/Alert'
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

  const fetchData = () => {
    fetch(
      process.env.REACT_APP_API_URL +
        '/api/releases/health?release=' +
        encodeURIComponent(props.release)
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
    if (row.releaseTime && row.releaseTime != '') {
      text = relativeTime(new Date(row.releaseTime))
      const when = new Date().getTime() - new Date(row.releaseTime).getTime()
      if (when <= 24 * 60 * 60 * 1000) {
        bgColor = theme.palette.success.light
        icon = <CheckCircle style={{ fill: 'green' }} />
      } else {
        bgColor = theme.palette.error.light
        icon = <Error style={{ fill: 'maroon' }} />
      }
    }

    let filter = {
      items: [
        filterFor('architecture', 'equals', row.architecture),
        filterFor('stream', 'equals', row.stream),
      ],
    }

    cards.push(
      <Box
        component={Link}
        to={`/release/${props.release}/tags?filters=${encodeURIComponent(
          JSON.stringify(filter)
        )}`}
      >
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
