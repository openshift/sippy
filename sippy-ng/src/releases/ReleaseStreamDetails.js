import { Alert } from '@material-ui/lab'
import { Container, Grid, makeStyles, Typography } from '@material-ui/core'
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

  const [release = props.release, setRelease] = useQueryParam(
    'release',
    StringParam
  )
  const [arch = props.arch, setArch] = useQueryParam('arch', StringParam)
  const [stream = props.stream, setStream] = useQueryParam(
    'stream',
    StringParam
  )

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
