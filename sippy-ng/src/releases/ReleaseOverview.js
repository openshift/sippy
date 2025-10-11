import { Box, Card, Container, Tooltip, Typography } from '@mui/material'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { ReleasesContext } from '../App'
import { useGlobalChat } from '../chat/useGlobalChat'
import Alert from '@mui/material/Alert'
import Grid from '@mui/material/Grid'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import ReleasePayloadAcceptance from './ReleasePayloadAcceptance'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TopLevelIndicators from './TopLevelIndicators'

const useStyles = makeStyles((theme) => ({
  root: {
    flexGrow: 1,
  },
  card: {
    minWidth: 275,
    alignContent: 'center',
    margin: 'auto',
  },
  title: {
    textAlign: 'center',
  },
  warning: {
    margin: 10,
    width: '100%',
  },
}))

export default function ReleaseOverview(props) {
  const classes = useStyles()
  const { updatePageContext } = useGlobalChat()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const hasSetContextRef = React.useRef(false)
  const releases = React.useContext(ReleasesContext)

  const fetchData = () => {
    fetch(
      process.env.REACT_APP_API_URL + '/api/health?release=' + props.release
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setData(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve release ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Health Summary`
    fetchData()
  }, [])

  // Update page context for chat
  useEffect(() => {
    if (!isLoaded || !data.indicators || hasSetContextRef.current) return

    hasSetContextRef.current = true
    updatePageContext({
      page: 'release-overview',
      url: window.location.href,
      suggestedQuestions: ['How is the overall health of the release?'],
      data: {
        release: props.release,
        indicators: {
          infrastructure: data.indicators.infrastructure
            ? {
                current_pass_percentage:
                  data.indicators.infrastructure.current_working_percentage,
                current_runs: data.indicators.infrastructure.current_runs,
                previous_pass_percentage:
                  data.indicators.infrastructure.previous_working_percentage,
                previous_runs: data.indicators.infrastructure.previous_runs,
                net_improvement:
                  data.indicators.infrastructure.net_working_improvement,
              }
            : null,
          install: data.indicators.install
            ? {
                current_pass_percentage:
                  data.indicators.install.current_working_percentage,
                current_runs: data.indicators.install.current_runs,
                previous_pass_percentage:
                  data.indicators.install.previous_working_percentage,
                previous_runs: data.indicators.install.previous_runs,
                net_improvement:
                  data.indicators.install.net_working_improvement,
              }
            : null,
          tests: data.indicators.tests
            ? {
                current_pass_percentage:
                  data.indicators.tests.current_working_percentage,
                current_runs: data.indicators.tests.current_runs,
                previous_pass_percentage:
                  data.indicators.tests.previous_working_percentage,
                previous_runs: data.indicators.tests.previous_runs,
                net_improvement: data.indicators.tests.net_working_improvement,
              }
            : null,
          upgrade: data.indicators.upgrade
            ? {
                current_pass_percentage:
                  data.indicators.upgrade.current_working_percentage,
                current_runs: data.indicators.upgrade.current_runs,
                previous_pass_percentage:
                  data.indicators.upgrade.previous_working_percentage,
                previous_runs: data.indicators.upgrade.previous_runs,
                net_improvement:
                  data.indicators.upgrade.net_working_improvement,
              }
            : null,
        },
        statistics: {
          current_mean: data.current_statistics?.mean,
          previous_mean: data.previous_statistics?.mean,
          quartiles: data.current_statistics?.quartiles,
          standard_deviation: data.current_statistics?.standard_deviation,
        },
      },
    })

    // Cleanup: Clear context when component unmounts
    return () => {
      updatePageContext(null)
    }
  }, [isLoaded, updatePageContext])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  const warnings = []
  if (data.warnings && data.warnings.length > 0) {
    data.warnings.forEach((warning, index) => {
      warnings.push(
        <Alert
          key={'sippy-warning-' + index}
          className={classes.warning}
          severity="warning"
        >
          <div
            style={{ width: '100%' }}
            dangerouslySetInnerHTML={{ __html: warning }}
          ></div>
        </Alert>
      )
    })
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} />
      <div style={{ position: 'relative' }}>
        <div
          style={{
            position: 'absolute',
            top: 20,
            right: 20,
            zIndex: 1000,
          }}
        ></div>
      </div>
      <div className="{classes.root}" style={{ padding: 20 }}>
        <Container maxWidth="lg">
          <Typography variant="h4" gutterBottom className={classes.title}>
            {props.release} Overview
          </Typography>
          <Grid container spacing={3} alignItems="stretch">
            {warnings}
            <TopLevelIndicators
              release={props.release}
              indicators={data.indicators}
              releases={releases}
            />

            {releases?.release_attrs?.[props.release]?.capabilities
              ?.payloadTags && (
              <Grid item md={12}>
                <Card elevation={5} style={{ padding: 20 }}>
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      mb: 2,
                    }}
                  >
                    <Typography variant="h5">
                      <Link to={`/release/${props.release}/tags`}>
                        Payload Acceptance
                      </Link>
                    </Typography>
                    <Tooltip
                      title={
                        'These cards show the last accepted payload for each architecture/stream combination.'
                      }
                    >
                      <InfoIcon sx={{ ml: 1, fontSize: 20, opacity: 0.6 }} />
                    </Tooltip>
                  </Box>

                  <Grid
                    container
                    spacing={3}
                    justifyContent="flex-start"
                    alignItems="stretch"
                  >
                    <ReleasePayloadAcceptance release={props.release} />
                  </Grid>
                </Card>
              </Grid>
            )}
          </Grid>
        </Container>
      </div>
    </Fragment>
  )
}

ReleaseOverview.propTypes = {
  release: PropTypes.string.isRequired,
}
