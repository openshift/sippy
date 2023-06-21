import './App.css'
import { CompReadyVarsProvider } from './component_readiness/CompReadyVars'
import { createTheme, makeStyles, useTheme } from '@material-ui/core/styles'
import { CssBaseline, Grid, MuiThemeProvider } from '@material-ui/core'
import { getReportStartDate, relativeTime } from './helpers'
import { JobAnalysis } from './jobs/JobAnalysis'
import { parse, stringify } from 'query-string'
import { QueryParamProvider } from 'use-query-params'
import { ReactRouter5Adapter } from 'use-query-params/adapters/react-router-5'
import { Route, Switch } from 'react-router-dom'
import { TestAnalysis } from './tests/TestAnalysis'
import Alert from '@material-ui/lab/Alert'
import AppBar from '@material-ui/core/AppBar'
import BuildClusterDetails from './build_clusters/BuildClusterDetails'
import BuildClusterOverview from './build_clusters/BuildClusterOverview'
import ChevronLeftIcon from '@material-ui/icons/ChevronLeft'
import ChevronRightIcon from '@material-ui/icons/ChevronRight'
import clsx from 'clsx'
import ComponentReadiness from './component_readiness/ComponentReadiness'
import Drawer from '@material-ui/core/Drawer'
import IconButton from '@material-ui/core/IconButton'
import Install from './releases/Install'
import Jobs from './jobs/Jobs'
import MenuIcon from '@material-ui/icons/Menu'
import PayloadStream from './releases/PayloadStream'
import PayloadStreams from './releases/PayloadStreams'
import ProwJobRun from './prow_job_runs/ProwJobRun'
import PullRequests from './pull_requests/PullRequests'
import PullRequestsTable from './pull_requests/PullRequestsTable'
import React, { useEffect } from 'react'
import ReleaseOverview from './releases/ReleaseOverview'
import ReleasePayloadDetails from './releases/ReleasePayloadDetails'
import ReleasePayloads from './releases/ReleasePayloads'
import Repositories from './repositories/Repositories'
import RepositoryDetails from './repositories/RepositoryDetails'
import Sidebar from './components/Sidebar'
import Tests from './tests/Tests'
import Toolbar from '@material-ui/core/Toolbar'
import Typography from '@material-ui/core/Typography'
import Upgrades from './releases/Upgrades'
import VariantStatus from './jobs/VariantStatus'

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

export const CapabilitiesContext = React.createContext([])
export const ReportEndContext = React.createContext('')

export default function App(props) {
  const classes = useStyles()
  const theme = useTheme()

  const [lastUpdated, setLastUpdated] = React.useState(null)
  const [drawerOpen, setDrawerOpen] = React.useState(true)
  const [isLoaded, setLoaded] = React.useState(false)
  const [releases, setReleases] = React.useState([])
  const [capabilities, setCapabilities] = React.useState([])
  const [reportDate, setReportDate] = React.useState([])
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    Promise.all([
      fetch(process.env.REACT_APP_API_URL + '/api/releases'),
      fetch(process.env.REACT_APP_API_URL + '/api/capabilities'),
      fetch(process.env.REACT_APP_API_URL + '/api/report_date'),
    ])
      .then(([releases, capabilities, reportDate]) => {
        if (releases.status !== 200) {
          throw new Error('server returned ' + releases.status)
        }

        if (capabilities.status !== 200) {
          throw new Error('server returned ' + capabilities.status)
        }

        if (reportDate.status !== 200) {
          throw new Error('server returned ' + reportDate.status)
        }

        return Promise.all([
          releases.json(),
          capabilities.json(),
          reportDate.json(),
        ])
      })
      .then(([releases, capabilities, reportDate]) => {
        setReleases(releases['releases'])
        setCapabilities(capabilities)
        setReportDate(reportDate['pinnedDateTime'])
        setLastUpdated(new Date(releases.last_updated))
        setLoaded(true)
      })
      .catch((error) => {
        setLoaded(true)
        setFetchError('could not retrieve data:' + error)
      })
  }

  useEffect(() => {
    if (!isLoaded) {
      fetchData()
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

  const startDate = getReportStartDate(reportDate)
  return (
    <ReportEndContext.Provider value={reportDate}>
      <CapabilitiesContext.Provider value={capabilities}>
        <MuiThemeProvider theme={createTheme(lightMode)}>
          <CssBaseline />
          <QueryParamProvider
            ReactRouterRoute={Route}
            adapter={ReactRouter5Adapter}
            options={{
              searchStringToObject: parse,
              objectToSearchString: stringify,
            }}
          >
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
                    className={clsx(
                      classes.menuButton,
                      drawerOpen && classes.hide
                    )}
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
                    Last updated{' '}
                    {lastUpdated !== null
                      ? relativeTime(lastUpdated, startDate)
                      : 'unknown'}
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
                    path="/release/:release/streams/:arch/:stream"
                    render={(props) => (
                      <PayloadStream
                        release={props.match.params.release}
                        arch={props.match.params.arch}
                        stream={props.match.params.stream}
                      />
                    )}
                  />

                  <Route
                    path="/release/:release/streams"
                    render={(props) => (
                      <PayloadStreams
                        key={'release-streams-' + props.match.params.release}
                        release={props.match.params.release}
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
                    path="/component_readiness"
                    render={(props) => {
                      return (
                        <CompReadyVarsProvider>
                          <ComponentReadiness />
                        </CompReadyVarsProvider>
                      )
                    }}
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

                  <Route
                    path="/build_clusters/:cluster"
                    render={(props) => (
                      <BuildClusterDetails
                        key={'cluster-' + props.match.params.cluster}
                        cluster={props.match.params.cluster}
                      />
                    )}
                  />

                  <Route
                    path="/build_clusters"
                    render={() => <BuildClusterOverview />}
                  />

                  <Route
                    path="/repositories/:release/:org/:repo"
                    render={(props) => (
                      <RepositoryDetails
                        release={props.match.params.release}
                        org={props.match.params.org}
                        repo={props.match.params.repo}
                      />
                    )}
                  />

                  <Route
                    path="/repositories/:release"
                    render={(props) => (
                      <Repositories release={props.match.params.release} />
                    )}
                  />

                  <Route
                    path="/pull_requests/:release"
                    render={(props) => (
                      <PullRequests
                        key={'pr-' + props.match.params.release}
                        release={props.match.params.release}
                      />
                    )}
                  />

                  <Route
                    path="/job_runs/:jobrunid/intervals"
                    render={(props) => (
                      <ProwJobRun jobRunID={props.match.params.jobrunid} />
                    )}
                  />

                  <Route path="/">{landingPage}</Route>
                </Switch>
                {/* eslint-enable react/prop-types */}
              </main>
            </div>
          </QueryParamProvider>
        </MuiThemeProvider>
      </CapabilitiesContext.Provider>
    </ReportEndContext.Provider>
  )
}
