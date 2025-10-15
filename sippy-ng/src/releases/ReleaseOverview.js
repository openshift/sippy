import { BOOKMARKS } from '../constants'
import { CapabilitiesContext } from '../App'
import { Card, Container, Tooltip, Typography } from '@mui/material'
import { dayFilter, JobStackedChart } from '../jobs/JobStackedChart'
import {
  getReportStartDate,
  pathForJobsWithFilter,
  queryForBookmark,
  safeEncodeURIComponent,
  withoutUnstable,
  withSort,
} from '../helpers'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { NumberParam, useQueryParam } from 'use-query-params'
import { ReportEndContext } from '../App'
import { usePageContextForChat } from '../chat/store/useChatStore'
import Alert from '@mui/material/Alert'
import AskSippyButton from '../chat/AskSippyButton'
import Grid from '@mui/material/Grid'
import Histogram from '../components/Histogram'
import InfoIcon from '@mui/icons-material/Info'
import JobTable from '../jobs/JobTable'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import ReleasePayloadAcceptance from './ReleasePayloadAcceptance'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestTable from '../tests/TestTable'
import TopLevelIndicators from './TopLevelIndicators'
import VariantCards from '../jobs/VariantCards'

export const REGRESSED_TOOLTIP =
  'Shows the most regressed items this week vs. last week, for those with more than 10 runs, excluding never-stable.'
export const TWODAY_WARNING =
  'Shows the last 2 days compared to the last 7 days, sorted by most regressed, excluding never-stable.'
export const TOP_FAILERS_TOOLTIP =
  'Shows the list of tests ordered by their failure percentage.'

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
  const { setPageContextForChat, unsetPageContextForChat } =
    usePageContextForChat()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [dayOffset = 1, setDayOffset] = useQueryParam('dayOffset', NumberParam)
  const startDate = getReportStartDate(React.useContext(ReportEndContext))
  const hasSetContextRef = React.useRef(false)

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
        >
          <AskSippyButton
            question="How is the overall health of the release?"
            tooltip="Ask Sippy about this release"
          />
        </div>
      </div>
      <div className="{classes.root}" style={{ padding: 20 }}>
        <Container maxWidth="lg">
          <Typography variant="h4" gutterBottom className={classes.title}>
            CI Release {props.release} Health Summary
          </Typography>
          <Grid container spacing={3} alignItems="stretch">
            {warnings}
            <TopLevelIndicators
              release={props.release}
              indicators={data.indicators}
            />

            {data && data.current_statistics && data.previous_statistics && (
              <Grid item md={5} sm={12}>
                <Card elevation={5} style={{ padding: 20, height: '100%' }}>
                  <Typography variant="h6">
                    <Link
                      to={withSort(
                        pathForJobsWithFilter(props.release, {
                          items: withoutUnstable(),
                        }),
                        'current_pass_percentage',
                        'asc'
                      )}
                    >
                      Job histogram
                    </Link>
                    <Tooltip
                      title={
                        'Histogram of job pass rates for frequently running jobs. Bucketed by current period pass percentage. ' +
                        'Tech preview and never-stable jobs are excluded. The solid line indicates the current ' +
                        "period's mean, and the dashed line is the previous period."
                      }
                    >
                      <InfoIcon />
                    </Tooltip>
                  </Typography>
                  <Histogram
                    data={data.current_statistics.histogram}
                    current_mean={data.current_statistics.mean}
                    previous_mean={data.previous_statistics.mean}
                    release={props.release}
                  />
                  {data.current_statistics.quartiles && (
                    <div align="center">
                      <span style={{ marginRight: 10 }}>
                        1Q: {data.current_statistics.quartiles[0].toFixed(0)}%
                      </span>
                      <span style={{ marginRight: 10 }}>
                        2Q: {data.current_statistics.quartiles[1].toFixed(0)}%
                      </span>
                      <span style={{ marginRight: 10 }}>
                        3Q: {data.current_statistics.quartiles[2].toFixed(0)}%
                      </span>
                      <span style={{ marginRight: 10 }}>
                        SD:{' '}
                        {data.current_statistics.standard_deviation.toFixed(2)}
                      </span>
                    </div>
                  )}
                </Card>
              </Grid>
            )}

            <Grid item md={7}>
              <Card elevation={5} style={{ padding: 20, height: '100%' }}>
                <Typography variant="h6">
                  <Link
                    to={`/jobs/${
                      props.release
                    }/analysis?filters=${safeEncodeURIComponent(
                      JSON.stringify({
                        items: [
                          ...withoutUnstable(),
                          ...dayFilter(14, startDate),
                        ],
                        linkOperator: 'and',
                      })
                    )}&period=day}`}
                  >
                    Last 14 days
                  </Link>
                  <Tooltip
                    title={
                      'This chart shows a 14 day period of job runs, excluding never-stable and tech preview. '
                    }
                  >
                    <InfoIcon />
                  </Tooltip>
                </Typography>
                <JobStackedChart
                  release={props.release}
                  period="day"
                  filter={{
                    items: [...withoutUnstable(), ...dayFilter(14, startDate)],
                    linkOperator: 'and',
                  }}
                />
              </Card>
            </Grid>

            <CapabilitiesContext.Consumer>
              {(value) => {
                if (!value.includes('openshift_releases')) {
                  return
                }

                return (
                  <Grid item md={12}>
                    <Typography style={{ textAlign: 'left' }} variant="h5">
                      <Link to={`/release/${props.release}/tags`}>
                        Payload acceptance
                      </Link>
                      <Tooltip
                        title={
                          'These cards show the last accepted payload for each architecture/stream combination.'
                        }
                      >
                        <InfoIcon />
                      </Tooltip>
                    </Typography>

                    <Card
                      elevation={5}
                      style={{
                        width: '100%',
                        padding: 10,
                        marginRight: 20,
                        margin: 10,
                      }}
                    >
                      <Grid
                        container
                        spacing={3}
                        justifyContent="center"
                        alignItems="center"
                      >
                        <ReleasePayloadAcceptance release={props.release} />
                      </Grid>
                    </Card>
                  </Grid>
                )
              }}
            </CapabilitiesContext.Consumer>

            <Grid item md={12}>
              <VariantCards release={props.release} />
            </Grid>

            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={`/jobs/${
                    props.release
                  }?sortField=net_improvement&sort=asc&${queryForBookmark(
                    BOOKMARKS.RUN_7,
                    BOOKMARKS.NO_STEP_GRAPH,
                    ...withoutUnstable()
                  )}`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Most regressed jobs
                  <Tooltip title={REGRESSED_TOOLTIP}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <JobTable
                  hideControls={true}
                  sortField="net_improvement"
                  sort="asc"
                  limit={10}
                  rowsPerPageOptions={[5]}
                  filterModel={{
                    items: [
                      BOOKMARKS.RUN_7,
                      BOOKMARKS.NO_NEVER_STABLE,
                      BOOKMARKS.NO_STEP_GRAPH,
                    ],
                  }}
                  pageSize={5}
                  release={props.release}
                  briefTable={true}
                />
              </Card>
            </Grid>
            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={`/jobs/${
                    props.release
                  }?period=twoDay&sortField=net_improvement&sort=asc&${queryForBookmark(
                    BOOKMARKS.RUN_2,
                    ...withoutUnstable()
                  )}`}
                  variant="h5"
                >
                  Most regressed jobs (two day)
                  <Tooltip title={TWODAY_WARNING}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <JobTable
                  hideControls={true}
                  sortField="net_improvement"
                  sort="asc"
                  limit={10}
                  rowsPerPageOptions={[5]}
                  filterModel={{
                    items: [BOOKMARKS.RUN_2, ...withoutUnstable()],
                  }}
                  pageSize={5}
                  period="twoDay"
                  release={props.release}
                  briefTable={true}
                />
              </Card>
            </Grid>

            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={`/tests/${props.release}?${queryForBookmark(
                    BOOKMARKS.RUN_7,
                    BOOKMARKS.NO_NEVER_STABLE,
                    BOOKMARKS.NO_AGGREGATED,
                    BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                    BOOKMARKS.NO_STEP_GRAPH
                  )}&sortField=net_improvement&sort=asc`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Most regressed tests
                  <Tooltip title={REGRESSED_TOOLTIP}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <Container size="xl">
                  <TestTable
                    hideControls={true}
                    sortField="net_improvement"
                    sort="asc"
                    limit={10}
                    rowsPerPageOptions={[5]}
                    filterModel={{
                      items: [
                        BOOKMARKS.RUN_7,
                        BOOKMARKS.NO_NEVER_STABLE,
                        BOOKMARKS.NO_AGGREGATED,
                        BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                        BOOKMARKS.NO_STEP_GRAPH,
                        BOOKMARKS.NO_100_FLAKE,
                      ],
                    }}
                    pageSize={5}
                    briefTable={true}
                    release={props.release}
                  />
                </Container>
              </Card>
            </Grid>

            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={`/tests/${
                    props.release
                  }?period=twoDay&sortField=net_improvement&sort=asc&${queryForBookmark(
                    BOOKMARKS.RUN_2,
                    BOOKMARKS.NO_NEVER_STABLE,
                    BOOKMARKS.NO_AGGREGATED,
                    BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                    BOOKMARKS.NO_STEP_GRAPH,
                    BOOKMARKS.NO_100_FLAKE
                  )}`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Most regressed tests (two day)
                  <Tooltip title={TWODAY_WARNING}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>
                <Container size="xl">
                  <TestTable
                    hideControls={true}
                    sortField="net_improvement"
                    sort="asc"
                    limit={10}
                    rowsPerPageOptions={[5]}
                    filterModel={{
                      items: [
                        BOOKMARKS.RUN_2,
                        BOOKMARKS.NO_NEVER_STABLE,
                        BOOKMARKS.NO_AGGREGATED,
                        BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                        BOOKMARKS.NO_STEP_GRAPH,
                      ],
                    }}
                    pageSize={5}
                    period="twoDay"
                    release={props.release}
                    briefTable={true}
                  />
                </Container>
              </Card>
            </Grid>

            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={`/tests/${props.release}/details?${queryForBookmark(
                    BOOKMARKS.RUN_7,
                    BOOKMARKS.NO_NEVER_STABLE,
                    BOOKMARKS.NO_AGGREGATED,
                    BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                    BOOKMARKS.NO_STEP_GRAPH,
                    BOOKMARKS.HIGH_DELTA_FROM_PASSING_AVERAGE,
                    BOOKMARKS.HIGH_STANDARD_DEVIATION,
                    BOOKMARKS.NO_100_FLAKE
                  )}&sortField=delta_from_passing_average&sort=asc`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Top failing test NURPs
                  <Tooltip
                    title={
                      'Show the list of tests with a variant that perform significantly worse than the other variants of the same tests.'
                    }
                  >
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <Container size="xl">
                  <TestTable
                    collapse={false}
                    overall={false}
                    hideControls={true}
                    sortField="delta_from_passing_average"
                    sort="asc"
                    limit={10}
                    rowsPerPageOptions={[5]}
                    filterModel={{
                      items: [
                        BOOKMARKS.RUN_7,
                        BOOKMARKS.NO_NEVER_STABLE,
                        BOOKMARKS.NO_AGGREGATED,
                        BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                        BOOKMARKS.NO_STEP_GRAPH,
                        BOOKMARKS.HIGH_DELTA_FROM_PASSING_AVERAGE,
                        BOOKMARKS.HIGH_STANDARD_DEVIATION,
                        BOOKMARKS.NO_100_FLAKE,
                      ],
                    }}
                    pageSize={5}
                    briefTable={true}
                    release={props.release}
                  />
                </Container>
              </Card>
            </Grid>

            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={`/tests/${props.release}?${queryForBookmark(
                    BOOKMARKS.RUN_7,
                    BOOKMARKS.NO_NEVER_STABLE,
                    BOOKMARKS.NO_AGGREGATED,
                    BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                    BOOKMARKS.NO_STEP_GRAPH,
                    BOOKMARKS.NO_100_FLAKE
                  )}&sortField=current_pass_percentage&sort=asc`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Top failing tests
                  <Tooltip title={TOP_FAILERS_TOOLTIP}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <Container size="xl">
                  <TestTable
                    hideControls={true}
                    sortField="current_pass_percentage"
                    sort="asc"
                    limit={10}
                    rowsPerPageOptions={[5]}
                    filterModel={{
                      items: [
                        BOOKMARKS.RUN_7,
                        BOOKMARKS.NO_NEVER_STABLE,
                        BOOKMARKS.NO_AGGREGATED,
                        BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                        BOOKMARKS.NO_STEP_GRAPH,
                        BOOKMARKS.NO_100_FLAKE,
                      ],
                    }}
                    pageSize={5}
                    briefTable={true}
                    release={props.release}
                  />
                </Container>
              </Card>
            </Grid>
          </Grid>
        </Container>
      </div>
    </Fragment>
  )
}

ReleaseOverview.propTypes = {
  release: PropTypes.string.isRequired,
}
