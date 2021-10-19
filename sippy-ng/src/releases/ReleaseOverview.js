import { ArrowBack, ArrowForward } from '@material-ui/icons'
import {
  BOOKMARKS,
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_THRESHOLDS,
  TEST_THRESHOLDS,
  UPGRADE_THRESHOLDS,
} from '../constants'
import {
  Box,
  Button,
  Card,
  Container,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { createTheme, makeStyles } from '@material-ui/core/styles'
import { hourFilter, JobStackedChart } from '../jobs/JobStackedChart'
import { Link } from 'react-router-dom'
import { NumberParam, useQueryParam } from 'use-query-params'
import {
  pathForJobsWithFilter,
  queryForBookmark,
  withoutUnstable,
  withSort,
} from '../helpers'
import Alert from '@material-ui/lab/Alert'
import Grid from '@material-ui/core/Grid'
import Histogram from '../components/Histogram'
import InfoIcon from '@material-ui/icons/Info'
import JobTable from '../jobs/JobTable'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestTable from '../tests/TestTable'
import TopLevelIndicators from './TopLevelIndicators'
import VariantCards from '../jobs/VariantCards'

export const REGRESSED_TOOLTIP =
  'Shows the most regressed items this week vs. last week, for those with more than 10 runs, excluding never-stable and techpreview.'
export const NOBUG_TOOLTIP =
  'Shows the list of tests ordered by least successful and without a bug, for those with more than 10 runs'
export const TRT_TOOLTIP =
  'Shows a curated list of tests selected by the TRT team'
export const TWODAY_WARNING =
  'Shows the last 2 days compared to the last 7 days, sorted by most regressed, excluding never-stable and techpreview.'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
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
  }),
  { defaultTheme }
)

export default function ReleaseOverview(props) {
  const classes = useStyles()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [dayOffset = 1, setDayOffset] = useQueryParam('dayOffset', NumberParam)

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
                      'Histogram of job pass rates. Bucketed by current period pass percentage. ' +
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
                    SD: {data.current_statistics.standard_deviation.toFixed(2)}
                  </span>
                </div>
              </Card>
            </Grid>

            <Grid item md={7}>
              <Card elevation={5} style={{ padding: 20, height: '100%' }}>
                <Typography variant="h6">
                  <Link
                    to={`/jobs/${
                      props.release
                    }/analysis?filters=${encodeURIComponent(
                      JSON.stringify({
                        items: [
                          ...hourFilter(dayOffset),
                          BOOKMARKS.NO_TECHPREVIEW,
                          BOOKMARKS.NO_NEVER_STABLE,
                        ],
                        linkOperator: 'and',
                      })
                    )}&period=hour}`}
                  >
                    {dayOffset === 1
                      ? 'Last 24 hours'
                      : `${dayOffset * 24} hours ago`}
                  </Link>
                  <Tooltip
                    title={
                      'This chart shows a 24 hour period of  job runs, excluding never-stable and tech preview. ' +
                      'The most recent hour will generally look more successful until all jobs finish running because it will be heavily biased towards the shortest jobs(more likely to have already completed) which also tend to be the most reliable. ' +
                      'Use the arrow buttons below to move back and forward a day. All times are UTC.'
                    }
                  >
                    <InfoIcon />
                  </Tooltip>
                </Typography>
                <JobStackedChart
                  release={props.release}
                  period="hour"
                  filter={{
                    items: [
                      BOOKMARKS.NO_NEVER_STABLE,
                      BOOKMARKS.NO_TECHPREVIEW,
                      ...hourFilter(dayOffset),
                    ],
                    linkOperator: 'and',
                  }}
                />
                <div align="center">
                  <Button
                    onClick={() => setDayOffset(dayOffset + 1)}
                    startIcon={<ArrowBack />}
                  />
                  <Button
                    style={dayOffset > 1 ? {} : { display: 'none' }}
                    onClick={() => dayOffset > 1 && setDayOffset(dayOffset - 1)}
                    startIcon={<ArrowForward />}
                  />
                </div>
              </Card>
            </Grid>

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
                    BOOKMARKS.RUN_10,
                    BOOKMARKS.NO_NEVER_STABLE,
                    BOOKMARKS.NO_TECHPREVIEW,
                    BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE
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
                  filterModel={{
                    items: [
                      BOOKMARKS.RUN_10,
                      BOOKMARKS.NO_NEVER_STABLE,
                      BOOKMARKS.NO_TECHPREVIEW,
                      BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE,
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
                    BOOKMARKS.RUN_1,
                    BOOKMARKS.NO_NEVER_STABLE,
                    BOOKMARKS.NO_TECHPREVIEW
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
                  filterModel={{
                    items: [
                      BOOKMARKS.RUN_1,
                      BOOKMARKS.NO_NEVER_STABLE,
                      BOOKMARKS.NO_TECHPREVIEW,
                    ],
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
                    BOOKMARKS.RUN_10,
                    BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE
                  )}&sortField=net_improvement&sort=asc`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Most regressed tests
                  <Tooltip title={REGRESSED_TOOLTIP}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <TestTable
                  hideControls={true}
                  sortField="net_improvement"
                  sort="asc"
                  limit={10}
                  filterModel={{
                    items: [
                      BOOKMARKS.RUN_10,
                      BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE,
                    ],
                  }}
                  pageSize={5}
                  briefTable={true}
                  release={props.release}
                />
              </Card>
            </Grid>
            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={`/tests/${
                    props.release
                  }?period=twoDay&sortField=net_improvement&sort=asc&${queryForBookmark(
                    BOOKMARKS.RUN_1,
                    BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE
                  )}`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Most regressed tests (two day)
                  <Tooltip title={TWODAY_WARNING}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <TestTable
                  hideControls={true}
                  sortField="net_improvement"
                  sort="asc"
                  limit={10}
                  filterModel={{
                    items: [
                      BOOKMARKS.RUN_1,
                      BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE,
                    ],
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
                  to={`/tests/${
                    props.release
                  }?sortField=net_improvement&sort=asc&${queryForBookmark(
                    BOOKMARKS.RUN_10,
                    BOOKMARKS.NO_LINKED_BUG,
                    BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE
                  )}`}
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Top failing tests without a bug
                  <Tooltip title={NOBUG_TOOLTIP}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <TestTable
                  hideControls={true}
                  sortField="net_improvement"
                  sort="asc"
                  filterModel={{
                    items: [
                      BOOKMARKS.RUN_10,
                      BOOKMARKS.NO_LINKED_BUG,
                      BOOKMARKS.NO_MULTISTAGE_OR_TEMPLATE,
                    ],
                  }}
                  limit={10}
                  pageSize={5}
                  briefTable={true}
                  release={props.release}
                />
              </Card>
            </Grid>
            <Grid item md={6} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography
                  component={Link}
                  to={
                    '/tests/' +
                    props.release +
                    '?period=twoDay&sortField=net_improvement&sort=asc&filters=' +
                    encodeURIComponent(
                      JSON.stringify({ items: [BOOKMARKS.TRT] })
                    )
                  }
                  style={{ textAlign: 'center' }}
                  variant="h5"
                >
                  Curated by TRT
                  <Tooltip title={TRT_TOOLTIP}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <TestTable
                  hideControls={true}
                  sortField="net_improvement"
                  sort="asc"
                  filterModel={{
                    items: [BOOKMARKS.RUN_10, BOOKMARKS.TRT],
                  }}
                  limit={10}
                  pageSize={5}
                  briefTable={true}
                  release={props.release}
                />
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
