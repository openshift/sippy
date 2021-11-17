import {
  Box,
  Button,
  Card,
  CardContent,
  Container,
  Grid,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { CheckCircle, CompareArrows, Error, Help } from '@material-ui/icons'
import { createTheme, makeStyles } from '@material-ui/core/styles'
import { DataGrid } from '@material-ui/data-grid'
import { filterFor, relativeTime } from '../helpers'
import { JsonParam, StringParam, useQueryParam } from 'use-query-params'
import { Link, useHistory } from 'react-router-dom'
import Alert from '@material-ui/lab/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

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
    const when = new Date().getTime() - new Date(row.releaseTime).getTime()
    let bgColor = theme.palette.error.light
    let icon = <Error style={{ fill: 'maroon' }} />
    if (when <= 24 * 60 * 60 * 1000) {
      bgColor = theme.palette.success.light
      icon = <CheckCircle style={{ fill: 'green' }} />
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
          className={`${classes.miniCard}`}
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
              {icon}&nbsp;{relativeTime(new Date(row.releaseTime))}
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
