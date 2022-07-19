import { Container, Grid, Tooltip, Typography } from '@material-ui/core'
import { Error } from '@material-ui/icons'
import {
  relativeDuration,
  relativeTime,
  safeEncodeURIComponent,
} from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { TEST_THRESHOLDS } from '../constants'
import { useTheme } from '@material-ui/core/styles'
import Alert from '@material-ui/lab/Alert'
import NumberCard from '../components/NumberCard'
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

  const fetchData = () => {
    let queryString = ''

    if (props.release !== '') {
      queryString += '&release=' + safeEncodeURIComponent(props.release)
    }

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/releases/health?' +
        queryString.substring(1)
    )
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
            console.log('Found our stream')
            console.log(stream)
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
      <Grid item md={12}>
        <Grid
          container
          spacing={3}
          alignItems="stretch"
          justifyContent="center"
        >
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
          <Grid item md={6}>
            <NumberCard
              title="Last Payload Accepted"
              number={relativeTime(new Date(streamHealth.release_time))}
              bgColor={
                new Date().getTime() -
                  new Date(streamHealth.release_time).getTime() >
                24 * 60 * 60 * 1000
                  ? theme.palette.error.light
                  : theme.palette.success.light
              }
            />
          </Grid>{' '}
        </Grid>
        <Grid
          container
          spacing={3}
          alignItems="stretch"
          justifyContent="center"
        >
          <Grid item md={3}>
            <NumberCard
              title="Overall"
              number={Number(
                relativeDuration(
                  streamHealth.acceptance_statistics.total.mean_seconds_between
                ).value
              ).toFixed(1)}
              caption={
                'mean ' +
                relativeDuration(
                  streamHealth.acceptance_statistics.total.mean_seconds_between
                ).units +
                ' between accepted payloads'
              }
            />
          </Grid>
          <Grid item md={3}>
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
