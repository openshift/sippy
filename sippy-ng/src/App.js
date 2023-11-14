import './App.css'
import { adaptV4Theme, createTheme, useTheme } from '@mui/material/styles'
import { CompReadyVarsProvider } from './component_readiness/CompReadyVars'
import {
  CssBaseline,
  Grid,
  StyledEngineProvider,
  ThemeProvider,
} from '@mui/material'
import { getReportStartDate, relativeTime } from './helpers'
import { JobAnalysis } from './jobs/JobAnalysis'
import { makeStyles, styled } from '@mui/styles'
import { parse, stringify } from 'query-string'
import { QueryParamProvider } from 'use-query-params'
import { ReactRouter5Adapter } from 'use-query-params/adapters/react-router-5'
import { Route, Switch } from 'react-router-dom'
import { TestAnalysis } from './tests/TestAnalysis'
import Alert from '@mui/lab/Alert'
import BuildClusterDetails from './build_clusters/BuildClusterDetails'
import BuildClusterOverview from './build_clusters/BuildClusterOverview'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import clsx from 'clsx'
import ComponentReadiness from './component_readiness/ComponentReadiness'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import Install from './releases/Install'
import Jobs from './jobs/Jobs'
import MenuIcon from '@mui/icons-material/Menu'
import MuiAppBar from '@mui/material/AppBar'
import PayloadStream from './releases/PayloadStream'
import PayloadStreams from './releases/PayloadStreams'
import ProwJobRun from './prow_job_runs/ProwJobRun'
import PullRequests from './pull_requests/PullRequests'
import React, { useEffect } from 'react'
import ReleaseOverview from './releases/ReleaseOverview'
import ReleasePayloadDetails from './releases/ReleasePayloadDetails'
import ReleasePayloads from './releases/ReleasePayloads'
import Repositories from './repositories/Repositories'
import RepositoryDetails from './repositories/RepositoryDetails'
import Sidebar from './components/Sidebar'
import Tests from './tests/Tests'
import Toolbar from '@mui/material/Toolbar'
import Typography from '@mui/material/Typography'
import Upgrades from './releases/Upgrades'
import VariantStatus from './jobs/VariantStatus'

const drawerWidth = 240

const Main = styled('main', { shouldForwardProp: (prop) => prop !== 'open' })(
  ({ theme, open }) => ({
    flexGrow: 1,
    padding: theme.spacing(3),
    transition: theme.transitions.create('margin', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
    marginLeft: `-${drawerWidth}px`,
    ...(open && {
      transition: theme.transitions.create('margin', {
        easing: theme.transitions.easing.easeOut,
        duration: theme.transitions.duration.enteringScreen,
      }),
      marginLeft: 0,
    }),
  })
)

const AppBar = styled(MuiAppBar, {
  shouldForwardProp: (prop) => prop !== 'open',
})(({ theme, open }) => ({
  transition: theme.transitions.create(['margin', 'width'], {
    easing: theme.transitions.easing.sharp,
    duration: theme.transitions.duration.leavingScreen,
  }),
  ...(open && {
    width: `calc(100% - ${drawerWidth}px)`,
    marginLeft: `${drawerWidth}px`,
    transition: theme.transitions.create(['margin', 'width'], {
      easing: theme.transitions.easing.easeOut,
      duration: theme.transitions.duration.enteringScreen,
    }),
  }),
}))

const DrawerHeader = styled('div')(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  padding: theme.spacing(0, 1),
  // necessary for content to be below app bar
  ...theme.mixins.toolbar,
  justifyContent: 'flex-end',
}))

export const ReleasesContext = React.createContext({})
export const CapabilitiesContext = React.createContext([])
export const ReportEndContext = React.createContext('')

const useStyles = makeStyles(() => ({
  root: {
    display: 'flex',
    flexGrow: 1,
  },
  title: {
    flexGrow: 1,
  },
}))

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
        setReleases(releases)
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
                open={drawerOpen}
                sx={{ bgcolor: '#3f51b5' }}
              >
                <Toolbar edge="start">
                  <IconButton
                    color="inherit"
                    aria-label="open drawer"
                    onClick={handleDrawerOpen}
                    edge="start"
                    sx={{ mr: 2, ...(drawerOpen && { display: 'none' }) }}
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
                sx={{
                  width: drawerWidth,
                  flexShrink: 0,
                  '& .MuiDrawer-paper': {
                    width: drawerWidth,
                    boxSizing: 'border-box',
                  },
                }}
                variant="persistent"
                anchor="left"
                open={drawerOpen}
              >
                <DrawerHeader>
                  <IconButton onClick={handleDrawerClose} size="large">
                    {theme.direction === 'ltr' ? (
                      <ChevronLeftIcon />
                    ) : (
                      <ChevronRightIcon />
                    )}
                  </IconButton>
                </DrawerHeader>
                <Sidebar releases={releases['releases']} />
              </Drawer>

              <Main open={drawerOpen}>
                <DrawerHeader />
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
                          <ComponentReadiness key={window.location.href} />
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
                    path="/job_runs/:jobrunid/:jobname?/:repoinfo?/:pullnumber?/intervals"
                    render={(props) => (
                      <ProwJobRun
                        jobRunID={props.match.params.jobrunid}
                        jobName={props.match.params.jobname}
                        repoInfo={props.match.params.repoinfo}
                        pullNumber={props.match.params.pullnumber}
                      />
                    )}
                  />

                  <Route path="/">{landingPage}</Route>
                </Switch>
                {/* eslint-enable react/prop-types */}
              </Main>
            </div>
          </QueryParamProvider>
        </CapabilitiesContext.Provider>
      </ReportEndContext.Provider>
    </ReleasesContext.Provider>
  )
}
