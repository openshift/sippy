import './App.css'
import { createTheme, makeStyles, useTheme } from '@material-ui/core/styles'
import { CssBaseline, Grid, MuiThemeProvider } from '@material-ui/core'
import { filterFor } from './helpers'
import { JobAnalysis } from './jobs/JobAnalysis'
import { QueryParamProvider } from 'use-query-params'
import { Route, Switch } from 'react-router-dom'
import { TestAnalysis } from './tests/TestAnalysis'
import Alert from '@material-ui/lab/Alert'
import AppBar from '@material-ui/core/AppBar'
import ChevronLeftIcon from '@material-ui/icons/ChevronLeft'
import ChevronRightIcon from '@material-ui/icons/ChevronRight'
import clsx from 'clsx'
import Drawer from '@material-ui/core/Drawer'
import IconButton from '@material-ui/core/IconButton'
import Install from './releases/Install'
import Jobs from './jobs/Jobs'
import LastUpdated from './components/LastUpdated'
import MenuIcon from '@material-ui/icons/Menu'
import React, { useEffect } from 'react'
import ReleaseOverview from './releases/ReleaseOverview'
import ReleasePayloadDetails from './releases/ReleasePayloadDetails'
import ReleasePayloads from './releases/ReleasePayloads'
import Sidebar from './components/Sidebar'
import Tests from './tests/Tests'
import Toolbar from '@material-ui/core/Toolbar'
import Typography from '@material-ui/core/Typography'
import Upgrades from './releases/Upgrades'
import VariantStatus from './jobs/VariantStatus'
import WorkloadMetricsOverview from './workloadmetrics/WorkloadMetricsOverview'

const drawerWidth = 240

const useStyles = makeStyles((theme) => ({
  root: {
    display: 'flex',
    flexGrow: 1,
  },
  title: {
    flexGrow: 1,
  },
  appBar: {
    transition: theme.transitions.create(['margin', 'width'], {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
  },
  appBarShift: {
    width: `calc(100% - ${drawerWidth}px)`,
    marginLeft: drawerWidth,
    transition: theme.transitions.create(['margin', 'width'], {
      easing: theme.transitions.easing.easeOut,
      duration: theme.transitions.duration.enteringScreen,
    }),
  },
  backdrop: {
    zIndex: theme.zIndex.drawer + 1,
    color: '#fff',
  },
  menuButton: {
    marginRight: theme.spacing(2),
  },
  hide: {
    display: 'none',
  },
  drawer: {
    width: drawerWidth,
    flexShrink: 0,
  },
  drawerPaper: {
    width: drawerWidth,
  },
  drawerHeader: {
    display: 'flex',
    alignItems: 'center',
    padding: theme.spacing(0, 1),
    // necessary for content to be below app bar
    ...theme.mixins.toolbar,
    justifyContent: 'flex-end',
  },
  content: {
    maxWidth: '100%',
    flexGrow: 1,
    padding: theme.spacing(3),
    transition: theme.transitions.create('margin', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
    marginLeft: -drawerWidth,
  },
  contentShift: {
    transition: theme.transitions.create('margin', {
      easing: theme.transitions.easing.easeOut,
      duration: theme.transitions.duration.enteringScreen,
    }),
    marginLeft: 0,
  },
}))

// Default theme:
const lightMode = {
  palette: {
    type: 'light',
  },
}

export default function App(props) {
  const classes = useStyles()
  const theme = useTheme()

  const [lastUpdated, setLastUpdated] = React.useState(Date.now())
  const [drawerOpen, setDrawerOpen] = React.useState(false)
  const [isLoaded, setLoaded] = React.useState(false)
  const [releases, setReleases] = React.useState([])
  const [fetchError, setFetchError] = React.useState('')

  const fetchReleases = () => {
    fetch(process.env.REACT_APP_API_URL + '/api/releases')
      .then((response) => {
        if (response.status !== 200) {
          setFetchError(
            'Could not retrieve releases, server returned ' + response.status
          )
        }
        return response.json()
      })
      .then((json) => {
        if (json.releases) {
          setReleases(json.releases)
          setLastUpdated(new Date(json.last_updated))
          setLoaded(true)
        } else {
          throw new Error('no releases found')
        }
      })
      .catch((error) => {
        setFetchError('Could not retrieve releases, ' + error)
        setLoaded(true)
      })
  }

  useEffect(() => {
    if (!isLoaded) {
      fetchReleases()
    }
  })

  const handleDrawerOpen = () => {
    setDrawerOpen(true)
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
  }

  if (!isLoaded) {
    return <Typography>Loading...</Typography>
  }

  let landingPage = ''
  if (fetchError !== '') {
    landingPage = <Alert severity="error">{fetchError}</Alert>
  } else if (releases.length > 0) {
    landingPage = <ReleaseOverview key={releases[0]} release={releases[0]} />
  } else {
    landingPage = 'No data.'
  }

  return (
    <MuiThemeProvider theme={createTheme(lightMode)}>
      <CssBaseline />
      <QueryParamProvider ReactRouterRoute={Route}>
        <div className={classes.root}>
          <AppBar
            position="fixed"
            className={clsx(classes.appBar, {
              [classes.appBarShift]: drawerOpen,
            })}
          >
            <Toolbar edge="start">
              <IconButton
                color="inherit"
                aria-label="open drawer"
                onClick={handleDrawerOpen}
                edge="start"
                className={clsx(classes.menuButton, drawerOpen && classes.hide)}
              >
                <MenuIcon />
              </IconButton>
              <Grid
                container
                justifyContent="space-between"
                alignItems="center"
              >
                <Typography variant="h6" className={classes.title}>
                  Sippy
                </Typography>
                <LastUpdated lastUpdated={lastUpdated} />
              </Grid>
            </Toolbar>
          </AppBar>

          <Drawer
            className={classes.drawer}
            variant="persistent"
            anchor="left"
            open={drawerOpen}
            classes={{
              paper: classes.drawerPaper,
            }}
          >
            <div className={classes.drawerHeader}>
              <IconButton onClick={handleDrawerClose}>
                {theme.direction === 'ltr' ? (
                  <ChevronLeftIcon />
                ) : (
                  <ChevronRightIcon />
                )}
              </IconButton>
            </div>
            <Sidebar releases={releases} />
          </Drawer>

          <main
            className={clsx(classes.content, {
              [classes.contentShift]: drawerOpen,
            })}
          >
            <div className={classes.drawerHeader} />

            {/* eslint-disable react/prop-types */}
            <Switch>
              <Route path="/about">
                <p>Hello, world!</p>
              </Route>

              <Route
                path="/workloadmetrics/:release"
                render={(props) => (
                  <WorkloadMetricsOverview
                    key={'workload-metrics-' + props.match.params.release}
                    release={props.match.params.release}
                  />
                )}
              />

              <Route
                path="/release/:release/tags/:tag"
                render={(props) => (
                  <ReleasePayloadDetails
                    key={'release-details-' + props.match.params.release}
                    release={props.match.params.release}
                    releaseTag={props.match.params.tag}
                  />
                )}
              />

              <Route
                path="/release/:release/tags"
                render={(props) => (
                  <ReleasePayloads
                    key={'release-tags-' + props.match.params.release}
                    release={props.match.params.release}
                  />
                )}
              />

              <Route
                path="/release/:release"
                render={(props) => (
                  <ReleaseOverview
                    key={'release-overview-' + props.match.params.release}
                    release={props.match.params.release}
                  />
                )}
              />

              <Route
                path="/variants/:release/:variant"
                render={(props) => (
                  <VariantStatus
                    release={props.match.params.release}
                    variant={props.match.params.variant}
                  />
                )}
              />

              <Route
                path="/jobs/:release/analysis"
                render={(props) => (
                  <JobAnalysis release={props.match.params.release} />
                )}
              />

              <Route
                path="/jobs/:release"
                render={(props) => (
                  <Jobs
                    key={'jobs-' + props.match.params.release}
                    title={'Job results for ' + props.match.params.release}
                    release={props.match.params.release}
                  />
                )}
              />

              <Route
                path="/tests/:release/analysis"
                render={(props) => (
                  <TestAnalysis release={props.match.params.release} />
                )}
              />

              <Route
                path="/tests/:release"
                render={(props) => (
                  <Tests
                    key={'tests-' + props.match.params.release}
                    release={props.match.params.release}
                  />
                )}
              />

              <Route
                path="/upgrade/:release"
                render={(props) => (
                  <Upgrades
                    key={'upgrades-' + props.match.params.release}
                    release={props.match.params.release}
                  />
                )}
              />

              <Route
                path="/install/:release"
                render={(props) => (
                  <Install
                    key={'install-' + props.match.params.release}
                    release={props.match.params.release}
                  />
                )}
              />

              <Route path="/">{landingPage}</Route>
            </Switch>
            {/* eslint-enable react/prop-types */}
          </main>
        </div>
      </QueryParamProvider>
    </MuiThemeProvider>
  )
}
