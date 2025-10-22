import { Card, Container, Tooltip } from '@mui/material'
import {
  apiFetch,
  getReportStartDate,
  relativeDuration,
  relativeTime,
  safeEncodeURIComponent,
} from '../helpers'
import { ReportEndContext } from '../App'
import { StringParam, useQueryParam } from 'use-query-params'
import { TEST_THRESHOLDS } from '../constants'
import { useTheme } from '@mui/material/styles'
import Alert from '@mui/material/Alert'
// https://github.com/mui/material-ui/issues/31244
import Grid from '@mui/material/Unstable_Grid2'
import NumberCard from '../components/NumberCard'
import PayloadMiniCalendar from './PayloadMiniCalendar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SummaryCard from '../components/SummaryCard'

function PayloadStreamOverview(props) {
  const theme = useTheme()

  const [release = props.release, setRelease] = useQueryParam(
    'release',
    StringParam
  )
  const [arch = props.arch, setArch] = useQueryParam('arch', StringParam)
  const [stream = props.stream, setStream] = useQueryParam(
    'stream',
    StringParam
  )

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [streamHealth, setStreamHealth] = React.useState([])

  const startDate = getReportStartDate(React.useContext(ReportEndContext))

  const fetchData = () => {
    let queryString = ''

    if (props.release !== '') {
      queryString += '&release=' + safeEncodeURIComponent(props.release)
    }

    apiFetch('/api/releases/health?' + queryString.substring(1))
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        // Find our specific stream's health in the list:
        for (const stream of json) {
          if (
            stream.architecture === props.arch &&
            stream.stream === props.stream
          ) {
            setStreamHealth(stream)
          }
        }
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
    return <Alert severity="error">Failed to load data, {fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <Container>
      <Grid container spacing={3} alignItems="stretch" justifyContent="center">
        <Grid item md={9}>
          <NumberCard
            title="Last Payload Accepted"
            size={3}
            number={relativeTime(
              new Date(streamHealth.release_time),
              startDate
            )}
            bgColor={
              startDate.getTime() -
                new Date(streamHealth.release_time).getTime() >
              24 * 60 * 60 * 1000
                ? theme.palette.error.light
                : theme.palette.success.light
            }
          />
        </Grid>
      </Grid>
      <Grid container spacing={3} alignItems="stretch" justifyContent="center">
        <Grid item md={3}>
          <SummaryCard
            key="payload-acceptance-summary-overall"
            threshold={TEST_THRESHOLDS}
            name="Overall"
            success={streamHealth.phase_counts.total.accepted}
            flakes={0}
            caption={
              <Fragment>
                <Tooltip
                  title={`${streamHealth.phase_counts.total.accepted} runs`}
                >
                  <span>
                    {Number(
                      (streamHealth.phase_counts.total.accepted /
                        (streamHealth.phase_counts.total.accepted +
                          streamHealth.phase_counts.total.rejected)) *
                        100
                    ).toFixed(2)}
                    % Accepted
                  </span>
                </Tooltip>
              </Fragment>
            }
            fail={streamHealth.phase_counts.total.rejected}
          />
        </Grid>
        <Grid item md={3}>
          <SummaryCard
            key="payload-acceptance-summary-currentweek"
            threshold={TEST_THRESHOLDS}
            name="This Week"
            success={streamHealth.phase_counts.current_week.accepted}
            flakes={0}
            caption={
              <Fragment>
                <Tooltip
                  title={`${streamHealth.phase_counts.current_week.accepted} runs`}
                >
                  <span>
                    {Number(
                      (streamHealth.phase_counts.current_week.accepted /
                        (streamHealth.phase_counts.current_week.accepted +
                          streamHealth.phase_counts.current_week.rejected)) *
                        100
                    ).toFixed(2)}
                    % Accepted
                  </span>
                </Tooltip>
              </Fragment>
            }
            fail={streamHealth.phase_counts.current_week.rejected}
          />
        </Grid>
        <Grid item md={3}>
          <NumberCard
            title="Payload Streak"
            number={streamHealth.count}
            caption={`have been ${streamHealth.last_phase}`}
            bgColor={
              streamHealth.last_phase === 'Rejected'
                ? theme.palette.error.light
                : theme.palette.success.light
            }
          />
        </Grid>
      </Grid>

      <Grid
        container
        spacing={3}
        direction="row"
        alignItems="stretch"
        justifyContent="center"
      >
        <Grid item md={3}>
          <Grid container spacing={3}>
            <Grid item md={12}>
              <NumberCard
                title="Overall"
                number={Number(
                  relativeDuration(
                    streamHealth.acceptance_statistics.total
                      .mean_seconds_between
                  ).value
                ).toFixed(1)}
                caption={
                  'mean ' +
                  relativeDuration(
                    streamHealth.acceptance_statistics.total
                      .mean_seconds_between
                  ).units +
                  ' between accepted payloads'
                }
              />
            </Grid>
            <Grid item md={12}>
              <NumberCard
                title="This Week"
                number={Number(
                  relativeDuration(
                    streamHealth.acceptance_statistics.current_week
                      .mean_seconds_between
                  ).value
                ).toFixed(1)}
                caption={
                  'mean ' +
                  relativeDuration(
                    streamHealth.acceptance_statistics.current_week
                      .mean_seconds_between
                  ).units +
                  ' between accepted payloads'
                }
              />
            </Grid>
          </Grid>
        </Grid>
        <Grid item md={6}>
          <Grid item style={{ height: '100%' }}>
            <Card elevation={5} style={{ padding: 20, height: '100%' }}>
              <PayloadMiniCalendar
                release={props.release}
                arch={props.arch}
                stream={props.stream}
              />
            </Card>
          </Grid>
        </Grid>
      </Grid>
    </Container>
  )
}

PayloadStreamOverview.defaultProps = {}

PayloadStreamOverview.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}

export default PayloadStreamOverview
