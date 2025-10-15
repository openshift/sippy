import {
  Box,
  Card,
  CircularProgress,
  Container,
  Tooltip,
  Typography,
} from '@mui/material'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { ReleasesContext } from '../App'
import { usePageContextForChat } from '../chat/store/useChatStore'
import Alert from '@mui/material/Alert'
import Grid from '@mui/material/Grid'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import ReleaseKeyDates from './ReleaseKeyDates'
import ReleasePayloadAcceptance from './ReleasePayloadAcceptance'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TopLevelIndicators from './TopLevelIndicators'

const useStyles = makeStyles((theme) => ({
  errorContainer: {
    marginTop: theme.spacing(4),
  },
  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    minHeight: '60vh',
  },
  loadingContent: {
    textAlign: 'center',
  },
  loadingText: {
    marginTop: theme.spacing(2),
  },
  pageWrapper: {
    paddingTop: theme.spacing(3),
    paddingBottom: theme.spacing(3),
    paddingLeft: theme.spacing(2.5),
    paddingRight: theme.spacing(2.5),
  },
  titleContainer: {
    marginBottom: theme.spacing(3),
  },
  titleCard: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    height: '100%',
  },
  title: {
    fontWeight: 600,
  },
  card: {
    padding: theme.spacing(2.5),
  },
  cardHeader: {
    display: 'flex',
    alignItems: 'center',
    marginBottom: theme.spacing(2),
  },
  cardTitle: {
    textDecoration: 'none',
    color: 'inherit',
  },
  infoIcon: {
    marginLeft: theme.spacing(1),
    fontSize: 20,
    opacity: 0.6,
  },
}))

export default function ReleaseOverview(props) {
  const classes = useStyles()
  const { setPageContextForChat, unsetPageContextForChat } =
    usePageContextForChat()

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
    setPageContextForChat({
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
      unsetPageContextForChat()
    }
  }, [isLoaded, setPageContextForChat, unsetPageContextForChat])

  if (fetchError !== '') {
    return (
      <Container maxWidth="lg" className={classes.errorContainer}>
        <Alert severity="error">{fetchError}</Alert>
      </Container>
    )
  }

  if (!isLoaded) {
    return (
      <Box className={classes.loadingContainer}>
        <Box className={classes.loadingContent}>
          <CircularProgress size={48} />
          <Typography
            variant="body1"
            color="text.secondary"
            className={classes.loadingText}
          >
            Loading release overview...
          </Typography>
        </Box>
      </Box>
    )
  }

  const warnings = []
  if (data.warnings && data.warnings.length > 0) {
    data.warnings.forEach((warning, index) => {
      warnings.push(
        <Grid item xs={12} key={'sippy-warning-' + index}>
          <Alert severity="warning">
            <div dangerouslySetInnerHTML={{ __html: warning }}></div>
          </Alert>
        </Grid>
      )
    })
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} />
      <Box className={classes.pageWrapper}>
        <Container maxWidth="lg">
          <Grid
            container
            spacing={3}
            alignItems="stretch"
            className={classes.titleContainer}
          >
            <Grid item xs={12} md={8}>
              <Box className={classes.titleCard}>
                <Typography variant="h4" className={classes.title}>
                  {props.release} Overview
                </Typography>
              </Box>
            </Grid>
            <ReleaseKeyDates release={props.release} releases={releases} />
          </Grid>

          <Grid container spacing={3} alignItems="stretch">
            {warnings}
            <TopLevelIndicators
              release={props.release}
              indicators={data.indicators}
              releases={releases}
            />

            {releases?.release_attrs?.[props.release]?.capabilities
              ?.payloadTags && (
              <Grid item xs={12}>
                <Card elevation={5} className={classes.card}>
                  <Box className={classes.cardHeader}>
                    <Typography
                      variant="h6"
                      component={Link}
                      to={`/release/${props.release}/tags`}
                      className={classes.cardTitle}
                    >
                      Payload Acceptance
                    </Typography>
                    <Tooltip
                      title={
                        'These cards show the last accepted payload for each architecture/stream combination.'
                      }
                    >
                      <InfoIcon className={classes.infoIcon} />
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
      </Box>
    </Fragment>
  )
}

ReleaseOverview.propTypes = {
  release: PropTypes.string.isRequired,
}
