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

export const ReleasesContext = React.createContext({})
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
      .then(([fetchedReleases, fetchedCapabilities, fetchedReportDate]) => {
        if (fetchedReleases.status !== 200) {
          throw new Error('server returned ' + fetchedReleases.status)
        }

        if (fetchedCapabilities.status !== 200) {
          throw new Error('server returned ' + fetchedCapabilities.status)
        }

        if (fetchedReportDate.status !== 200) {
          throw new Error('server returned ' + fetchedReportDate.status)
        }

        return Promise.all([
          fetchedReleases.json(),
          fetchedCapabilities.json(),
          fetchedReportDate.json(),
        ])
      })
      .then(([jsonReleases, jsonCapabilities, jsonReportDate]) => {
        setReleases(jsonReleases)
        setCapabilities(jsonCapabilities)
        setReportDate(jsonReportDate['pinnedDateTime'])
        setLastUpdated(new Date(jsonReleases.last_updated))
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
  } else if (releases?.releases?.length > 0) {
    landingPage = (
      <ReleaseOverview
        key={releases.releases[0]}
        release={releases.releases[0]}
      />
    )
  } else {
    landingPage = 'No releases found! Have you configured Sippy correctly?'
  }

  const startDate = getReportStartDate(reportDate)
  return (
    <ReleasesContext.Provider value={releases}>
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
                  <Sidebar releases={releases['releases']} />
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
                      render={(theProps) => (
                        <ReleasePayloadDetails
                          key={
                            'release-details-' + theProps.match.params.release
                          }
                          release={theProps.match.params.release}
                          releaseTag={theProps.match.params.tag}
                        />
                      )}
                    />

                    <Route
                      path="/release/:release/streams/:arch/:stream"
                      render={(theProps) => (
                        <PayloadStream
                          release={theProps.match.params.release}
                          arch={theProps.match.params.arch}
                          stream={theProps.match.params.stream}
                        />
                      )}
                    />

                    <Route
                      path="/release/:release/streams"
                      render={(theProps) => (
                        <PayloadStreams
                          key={
                            'release-streams-' + theProps.match.params.release
                          }
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/release/:release/tags"
                      render={(theProps) => (
                        <ReleasePayloads
                          key={'release-tags-' + theProps.match.params.release}
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/release/:release"
                      render={(theProps) => (
                        <ReleaseOverview
                          key={
                            'release-overview-' + theProps.match.params.release
                          }
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/variants/:release/:variant"
                      render={(theProps) => (
                        <VariantStatus
                          release={theProps.match.params.release}
                          variant={theProps.match.params.variant}
                        />
                      )}
                    />

                    <Route
                      path="/jobs/:release/analysis"
                      render={(theProps) => (
                        <JobAnalysis release={theProps.match.params.release} />
                      )}
                    />

                    <Route
                      path="/jobs/:release"
                      render={(theProps) => (
                        <Jobs
                          key={'jobs-' + theProps.match.params.release}
                          title={
                            'Job results for ' + theProps.match.params.release
                          }
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/tests/:release/analysis"
                      render={(theProps) => (
                        <TestAnalysis release={theProps.match.params.release} />
                      )}
                    />

                    <Route
                      path="/tests/:release"
                      render={(theProps) => (
                        <Tests
                          key={'tests-' + theProps.match.params.release}
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/upgrade/:release"
                      render={(theProps) => (
                        <Upgrades
                          key={'upgrades-' + theProps.match.params.release}
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/component_readiness"
                      render={(theProps) => {
                        return (
                          <CompReadyVarsProvider>
                            <ComponentReadiness key={window.location.href} />
                          </CompReadyVarsProvider>
                        )
                      }}
                    />

                    <Route
                      path="/install/:release"
                      render={(theProps) => (
                        <Install
                          key={'install-' + theProps.match.params.release}
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/build_clusters/:cluster"
                      render={(theProps) => (
                        <BuildClusterDetails
                          key={'cluster-' + theProps.match.params.cluster}
                          cluster={theProps.match.params.cluster}
                        />
                      )}
                    />

                    <Route
                      path="/build_clusters"
                      render={() => <BuildClusterOverview />}
                    />

                    <Route
                      path="/repositories/:release/:org/:repo"
                      render={(theProps) => (
                        <RepositoryDetails
                          release={theProps.match.params.release}
                          org={theProps.match.params.org}
                          repo={theProps.match.params.repo}
                        />
                      )}
                    />

                    <Route
                      path="/repositories/:release"
                      render={(theProps) => (
                        <Repositories release={theProps.match.params.release} />
                      )}
                    />

                    <Route
                      path="/pull_requests/:release"
                      render={(theProps) => (
                        <PullRequests
                          key={'pr-' + theProps.match.params.release}
                          release={theProps.match.params.release}
                        />
                      )}
                    />

                    <Route
                      path="/job_runs/:jobrunid/:jobname?/:repoinfo?/:pullnumber?/intervals"
                      render={(theProps) => (
                        <ProwJobRun
                          jobRunID={theProps.match.params.jobrunid}
                          jobName={theProps.match.params.jobname}
                          repoInfo={theProps.match.params.repoinfo}
                          pullNumber={theProps.match.params.pullnumber}
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
        </CapabilitiesContext.Provider>
      </ReportEndContext.Provider>
    </ReleasesContext.Provider>
  )
}
