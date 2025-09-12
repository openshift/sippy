import { AccessibilityModeProvider } from './components/AccessibilityModeProvider'
import { CompReadyVarsProvider } from './component_readiness/CompReadyVars'
import { createTheme, useTheme } from '@mui/material/styles'
import {
  CssBaseline,
  Grid,
  StyledEngineProvider,
  ThemeProvider,
  Tooltip,
} from '@mui/material'
import { cyan, green, orange, red } from '@mui/material/colors'
import { DarkMode, LightMode, ToggleOff, ToggleOn } from '@mui/icons-material'
import { ErrorBoundary } from 'react-error-boundary'
import {
  findFirstNonGARelease,
  getReportStartDate,
  getUrlWithoutParams,
  pathForTestSubstringByVariant,
  relativeTime,
} from './helpers'
import { JobAnalysis } from './jobs/JobAnalysis'
import { makeStyles, styled } from '@mui/styles'
import {
  Navigate,
  Route,
  Routes,
  useNavigate,
  useParams,
} from 'react-router-dom'
import { parse, stringify } from 'query-string'
import { QueryParamProvider } from 'use-query-params'
import { ReactRouter6Adapter } from 'use-query-params/adapters/react-router-6'
import { TestAnalysis } from './tests/TestAnalysis'
import { useCookies } from 'react-cookie'
import AccessibilityToggle from './components/AccessibilityToggle'
import Alert from '@mui/material/Alert'
import BuildClusterDetails from './build_clusters/BuildClusterDetails'
import BuildClusterOverview from './build_clusters/BuildClusterOverview'
import ChatAgent from './chat/ChatAgent'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import ComponentReadiness from './component_readiness/ComponentReadiness'
import Drawer from '@mui/material/Drawer'
import FeatureGates from './tests/FeatureGates'
import IconButton from '@mui/material/IconButton'
import Install from './releases/Install'
import IntervalsChart from './prow_job_runs/IntervalsChart'
import Jobs from './jobs/Jobs'
import MenuIcon from '@mui/icons-material/Menu'
import MuiAppBar from '@mui/material/AppBar'
import PayloadStream from './releases/PayloadStream'
import PayloadStreams from './releases/PayloadStreams'
import PullRequests from './pull_requests/PullRequests'
import React, { Fragment, useEffect } from 'react'
import ReleaseOverview from './releases/ReleaseOverview'
import ReleasePayloadDetails from './releases/ReleasePayloadDetails'
import ReleasePayloads from './releases/ReleasePayloads'
import Repositories from './repositories/Repositories'
import RepositoryDetails from './repositories/RepositoryDetails'
import Sidebar from './components/Sidebar'
import Tests from './tests/Tests'
import Toolbar from '@mui/material/Toolbar'
import Triage from './component_readiness/Triage'
import TriageList from './component_readiness/TriageList'
import Typography from '@mui/material/Typography'
import Upgrades from './releases/Upgrades'
import VariantStatus from './jobs/VariantStatus'

const drawerWidth = 240

const Main = styled('main', { shouldForwardProp: (prop) => prop !== 'open' })(
  ({ theme, open }) => ({
    flexGrow: 1,
    width: `100vw`,
    padding: theme.spacing(3),
    transition: theme.transitions.create('margin', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
    marginLeft: `-${drawerWidth}px`,
    ...(open && {
      width: `calc(100% - ${drawerWidth}px)`,
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
const ColorModeContext = React.createContext({
  toggleColorMode: () => {},
})

const useStyles = makeStyles(() => ({
  root: {
    display: 'flex',
    flexGrow: 1,
  },
  title: {
    flexGrow: 1,
  },
}))

const themes = {
  dark: {
    palette: {
      mode: 'dark',
    },
  },
  light: {
    palette: {
      mode: 'light',
      info: {
        main: cyan[500],
        light: cyan[300],
        dark: cyan[700],
      },
      success: {
        main: green[300],
        light: green[300],
        dark: green[700],
      },
      warning: {
        main: orange[500],
        light: orange[300],
        dark: orange[700],
      },
      error: {
        main: red[500],
        light: red[300],
        dark: red[700],
      },
    },
  },
}

// Default theme, restore settings from v4 color schemes. v5 is much darker.

// for redirecting paths for /<something>/latest to the first non-GA release
const RedirectLatestReleaseWrapper = (component) => {
  const { release } = useParams()
  const navigate = useNavigate()

  if (release === 'latest') {
    const releases = React.useContext(ReleasesContext)
    const defaultRelease = findFirstNonGARelease(releases)
    navigate('../' + defaultRelease, { relative: 'path', replace: true })
  }

  return component
}

/**
 * Wrapper components for React Router v6+ upgrade
 *
 * These wrapper components are needed because React Router v6 changed how route props work:
 * - In v5: Route components received props like `match.params` automatically
 * - In v6: Route components must use `useParams()` hook to access URL parameters
 *
 * These wrappers are using useParams() hook and pass them as props to the original components
 *
 * This approach allows us to upgrade to React Router v6 while keeping the original component
 * interfaces unchanged, minimizing code changes throughout the application.
 */
const ReleasePayloadDetailsWrapper = () => {
  const { release, tag } = useParams()

  return RedirectLatestReleaseWrapper(
    <ReleasePayloadDetails
      key={'release-details-' + release}
      release={release}
      releaseTag={tag}
    />
  )
}

const PayloadStreamWrapper = () => {
  const { release, arch, stream } = useParams()
  return RedirectLatestReleaseWrapper(
    <PayloadStream release={release} arch={arch} stream={stream} />
  )
}

const PayloadStreamsWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <PayloadStreams key={'release-streams-' + release} release={release} />
  )
}

const ReleasePayloadsWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <ReleasePayloads key={'release-tags-' + release} release={release} />
  )
}

const ReleaseOverviewWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <ReleaseOverview key={'release-overview-' + release} release={release} />
  )
}

const VariantStatusWrapper = () => {
  const { release, variant } = useParams()
  return RedirectLatestReleaseWrapper(
    <VariantStatus release={release} variant={variant} />
  )
}

const JobAnalysisWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(<JobAnalysis release={release} />)
}

const JobsWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <Jobs
      key={'jobs-' + release}
      title={'Job results for ' + release}
      release={release}
    />
  )
}

const FeatureGateRedirectWrapper = () => {
  let { release, feature_gate } = useParams()
  if (release === 'latest') {
    release = findFirstNonGARelease(React.useContext(ReleasesContext))
  }

  return (
    <Navigate
      to={pathForTestSubstringByVariant(release, 'FeatureGate:' + feature_gate)}
      replace
    />
  )
}

const FeatureGatesWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <FeatureGates key={'jobs-' + release} release={release} />
  )
}

const TestAnalysisWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(<TestAnalysis release={release} />)
}

const TestsWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <Tests key={'tests-' + release} release={release} />
  )
}

const TriageWrapper = () => {
  const { id } = useParams()

  return (
    <CompReadyVarsProvider>
      <Triage id={id} />
    </CompReadyVarsProvider>
  )
}

const TriageListWrapper = () => {
  return (
    <CompReadyVarsProvider>
      <TriageList />
    </CompReadyVarsProvider>
  )
}

const UpgradesWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <Upgrades key={'upgrades-' + release} release={release} />
  )
}

const ComponentReadinessWrapper = () => {
  return (
    <CompReadyVarsProvider>
      <ComponentReadiness
        key={getUrlWithoutParams([
          'regressedModal',
          'regressedModalTab',
          'regressedModalRow',
          'regressedModalPage',
          'regressedModalTestRow',
          'regressedModalTestPage',
        ])}
      />
    </CompReadyVarsProvider>
  )
}

const InstallWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <Install key={'install-' + release} release={release} />
  )
}

const BuildClusterDetailsWrapper = () => {
  const { cluster } = useParams()

  return <BuildClusterDetails key={'cluster-' + cluster} cluster={cluster} />
}

const RepositoryDetailsWrapper = () => {
  const { release, org, repo } = useParams()
  return RedirectLatestReleaseWrapper(
    <RepositoryDetails release={release} org={org} repo={repo} />
  )
}

const RepositoriesWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(<Repositories release={release} />)
}

const PullRequestsWrapper = () => {
  const { release } = useParams()
  return RedirectLatestReleaseWrapper(
    <PullRequests key={'pr-' + release} release={release} />
  )
}

const IntervalsChartWrapper = () => {
  const { jobrunid, jobname, repoinfo, pullnumber } = useParams()

  return (
    <IntervalsChart
      jobRunID={jobrunid}
      jobName={jobname}
      repoInfo={repoinfo}
      pullNumber={pullnumber}
    />
  )
}

export default function App(props) {
  const classes = useStyles()
  const theme = useTheme()

  const [cookies, setCookie] = useCookies([
    'sippyColorMode',
    'testTableDBSource',
  ])
  const colorModePreference = cookies['sippyColorMode']
  const systemPrefersDark = window.matchMedia(
    '(prefers-color-scheme: dark)'
  ).matches
  const [mode, setMode] = React.useState(
    colorModePreference === 'dark' || colorModePreference === 'light'
      ? colorModePreference
      : systemPrefersDark
      ? 'dark'
      : 'light'
  )

  const colorMode = React.useMemo(
    () => ({
      toggleColorMode: () => {
        setMode((prevMode) => {
          const newMode = prevMode === 'light' ? 'dark' : 'light'
          setCookie('sippyColorMode', newMode, {
            path: '/',
            sameSite: 'Strict',
            expires: new Date('3000-12-31'),
          })
          return newMode
        })
      },
    }),
    [setCookie]
  )

  const testTableDBSourcePreference = cookies['testTableDBSource']
  const [testTableDBSource, setTestTableDBSource] = React.useState(
    testTableDBSourcePreference
  )
  const testTableDBSourceToggle = React.useMemo(
    () => ({
      toggleTestTableDBSource: () => {
        setTestTableDBSource((prevSource) => {
          const newSource = prevSource === 'bigquery' ? 'postgres' : 'bigquery'
          setCookie('testTableDBSource', newSource, {
            path: '/',
            sameSite: 'Strict',
            expires: new Date('3000-12-31'),
          })
          return newSource
        })
      },
    }),
    [setCookie]
  )

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
        // Remove the Z from the ga_dates so that when Date objects are created,
        // the date is not converted to a local time zone.
        for (const key in releases.ga_dates) {
          if (releases.ga_dates[key]) {
            releases.ga_dates[key] = releases.ga_dates[key].replace('Z', '')
          }
        }
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

  // Disable console.log in production
  useEffect(() => {
    if (process.env.NODE_ENV === 'production') {
      console.log = function () {}
    }
  }, [])

  useEffect(() => {
    if (!isLoaded) {
      fetchData()
    }
  })

  const handleDrawerOpen = () => {
    setDrawerOpen(true)
  }

  const showWithCapability = (capability, el) => {
    if (capabilities.includes(capability)) {
      return el
    }

    return null
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
  }

  if (!isLoaded) {
    return <Typography>Loading...</Typography>
  }

  let landingPage = ''
  let defaultRelease = findFirstNonGARelease(releases)
  if (fetchError !== '') {
    landingPage = <Alert severity="error">{fetchError}</Alert>
  } else if (defaultRelease.length > 0) {
    landingPage = (
      <ReleaseOverview key={defaultRelease} release={defaultRelease} />
    )
  } else {
    landingPage = 'No releases found! Have you configured Sippy correctly?'
  }

  const startDate = getReportStartDate(reportDate)
  return (
    <ColorModeContext.Provider value={colorMode}>
      <ThemeProvider theme={createTheme(themes[mode])}>
        <StyledEngineProvider injectFirst>
          <ReleasesContext.Provider value={releases}>
            <ReportEndContext.Provider value={reportDate}>
              <CapabilitiesContext.Provider value={capabilities}>
                <AccessibilityModeProvider>
                  <CssBaseline />
                  <QueryParamProvider
                    adapter={ReactRouter6Adapter}
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
                            sx={{
                              mr: 2,
                              ...(drawerOpen && { display: 'none' }),
                            }}
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
                            {showWithCapability(
                              'local_db',
                              <Fragment>
                                Last updated{' '}
                                {lastUpdated !== null
                                  ? relativeTime(lastUpdated, startDate)
                                  : 'unknown'}
                              </Fragment>
                            )}
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
                          <Tooltip
                            title={
                              mode === 'dark'
                                ? 'Toggle light mode'
                                : 'Toggle dark mode'
                            }
                          >
                            <IconButton
                              sx={{ ml: 1 }}
                              onClick={colorMode.toggleColorMode}
                              color="inherit"
                            >
                              {mode === 'dark' ? <LightMode /> : <DarkMode />}
                            </IconButton>
                          </Tooltip>
                          <AccessibilityToggle />
                          <Tooltip
                            title={
                              testTableDBSource === 'bigquery'
                                ? 'BigQuery as test table DB source'
                                : 'Postgres as test table DB source'
                            }
                          >
                            <IconButton
                              sx={{ ml: 1 }}
                              onClick={
                                testTableDBSourceToggle.toggleTestTableDBSource
                              }
                              color="inherit"
                            >
                              {testTableDBSource === 'bigquery' ? (
                                <ToggleOff />
                              ) : (
                                <ToggleOn />
                              )}
                            </IconButton>
                          </Tooltip>
                          <IconButton onClick={handleDrawerClose} size="large">
                            {theme.direction === 'ltr' ? (
                              <ChevronLeftIcon />
                            ) : (
                              <ChevronRightIcon />
                            )}
                          </IconButton>
                        </DrawerHeader>
                        <Sidebar
                          releaseConfig={releases}
                          defaultRelease={defaultRelease}
                        />
                      </Drawer>

                      <Main open={drawerOpen}>
                        <DrawerHeader />
                        <ErrorBoundary
                          fallback={<h2>An unknown error has occurred.</h2>}
                        >
                          <Routes>
                            <Route
                              path="/release/:release/tags/:tag/*"
                              element={<ReleasePayloadDetailsWrapper />}
                            />

                            <Route
                              path="/release/:release/streams/:arch/:stream/*"
                              element={<PayloadStreamWrapper />}
                            />

                            <Route
                              path="/release/:release/streams"
                              element={<PayloadStreamsWrapper />}
                            />

                            <Route
                              path="/release/:release/tags"
                              element={<ReleasePayloadsWrapper />}
                            />

                            <Route
                              path="/release/:release"
                              element={<ReleaseOverviewWrapper />}
                            />

                            <Route
                              path="/variants/:release/:variant"
                              element={<VariantStatusWrapper />}
                            />

                            <Route
                              path="/jobs/:release/analysis"
                              element={<JobAnalysisWrapper />}
                            />

                            <Route
                              path="/jobs/:release/*"
                              element={<JobsWrapper />}
                            />

                            <Route
                              path="/feature_gates/:release/:feature_gate"
                              element={<FeatureGateRedirectWrapper />}
                            />

                            <Route
                              path="/feature_gates/:release"
                              element={<FeatureGatesWrapper />}
                            />

                            <Route
                              path="/tests/:release/analysis"
                              element={<TestAnalysisWrapper />}
                            />

                            <Route
                              path="/tests/:release/*"
                              element={<TestsWrapper />}
                            />

                            <Route
                              path="/triages/:id"
                              element={<TriageWrapper />}
                            />

                            <Route
                              path="/triages"
                              element={<TriageListWrapper />}
                            />

                            <Route
                              path="/upgrade/:release/*"
                              element={<UpgradesWrapper />}
                            />

                            <Route
                              path="/component_readiness/*"
                              element={<ComponentReadinessWrapper />}
                            />

                            <Route
                              path="/install/:release/*"
                              element={<InstallWrapper />}
                            />

                            <Route
                              path="/build_clusters/:cluster"
                              element={<BuildClusterDetailsWrapper />}
                            />

                            <Route
                              path="/build_clusters"
                              element={<BuildClusterOverview />}
                            />

                            <Route
                              path="/repositories/:release/:org/:repo"
                              element={<RepositoryDetailsWrapper />}
                            />

                            <Route
                              path="/repositories/:release"
                              element={<RepositoriesWrapper />}
                            />

                            <Route
                              path="/pull_requests/:release"
                              element={<PullRequestsWrapper />}
                            />

                            <Route
                              path="/job_runs/:jobrunid/:jobname?/:repoinfo?/:pullnumber?/intervals"
                              element={<IntervalsChartWrapper />}
                            />

                            {/* Only show /chat route if REACT_APP_CHAT_API_URL is set */}
                            {process.env.REACT_APP_CHAT_API_URL && (
                              <Route path="/chat" element={<ChatAgent />} />
                            )}

                            <Route
                              path="/"
                              element={
                                capabilities.includes('local_db') ? (
                                  landingPage
                                ) : (
                                  <Navigate
                                    to="/component_readiness/main"
                                    replace
                                  />
                                )
                              }
                            />
                          </Routes>
                        </ErrorBoundary>
                        {/* eslint-enable react/prop-types */}
                      </Main>
                    </div>
                  </QueryParamProvider>
                </AccessibilityModeProvider>
              </CapabilitiesContext.Provider>
            </ReportEndContext.Provider>
          </ReleasesContext.Provider>
        </StyledEngineProvider>
      </ThemeProvider>
    </ColorModeContext.Provider>
  )
}
