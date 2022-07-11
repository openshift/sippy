import { Alert } from '@material-ui/lab'
import {
  Card,
  Container,
  Grid,
  makeStyles,
  Typography,
} from '@material-ui/core'
import { filterFor, safeEncodeURIComponent } from '../helpers'
import { Fragment } from 'react'
import { Link, Route, Switch, useRouteMatch } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'
import ReleaseStreamAnalysis from './ReleaseStreamAnalysis'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

const useStyles = makeStyles((theme) => ({
  title: {
    textAlign: 'center',
  },
  backdrop: {
    zIndex: theme.zIndex.drawer + 1,
    color: '#fff',
  },
}))

export default function ReleaseStreamDetails(props) {
  const classes = useStyles()
  const { path, url } = useRouteMatch()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)

  const [analysis, setAnalysis] = React.useState({})

  const [release = props.release, setRelease] = useQueryParam(
    'release',
    StringParam
  )
  const [arch = props.arch, setArch] = useQueryParam('arch', StringParam)
  const [stream = props.stream, setStream] = useQueryParam(
    'stream',
    StringParam
  )

  const fetchData = () => {
    /*
    const filter = safeEncodeURIComponent(
      JSON.stringify({
        items: [filterFor('release_tag', 'equals', releaseTag)],
      })
    )
     */

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/releases/stream_analysis?release=${release}&arch=${arch}&stream=${stream}`
      ),
    ])
      .then(([analysis]) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        return Promise.all([analysis.json()])
      })
      .then(([analysis]) => {
        if (analysis.length === 0) {
          return (
            <Typography variant="h5">
              Analysis #{analysis} not found.
            </Typography>
          )
        }
        console.log('GOT ANALYSIS')
        console.log(analysis)

        setAnalysis(analysis)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve payload stream analysis for ' +
            props.release +
            ' ' +
            props.arch +
            ' ' +
            props.stream +
            ': ' +
            error
        )
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

  let currPage = props.arch + ' ' + props.stream

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={
          <Link to={`/release/${props.release}/streams`}>Payload Streams</Link>
        }
        // TODO
        currentPage={currPage}
      />
      <Container xl>
        <Typography variant="h4" gutterBottom className={classes.title}>
          Payload Stream Analysis
        </Typography>

        <Typography variant="h5" gutterBottom className={classes.title}>
          {arch} {stream}
        </Typography>

        <Typography variant="h6" gutterBottom className={classes.title}>
          Potential Test Blockers
        </Typography>
        <ReleaseStreamAnalysis
          release={props.release}
          stream={props.stream}
          arch={props.arch}
        />
      </Container>
    </Fragment>
  )
}

ReleaseStreamDetails.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}
