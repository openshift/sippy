import {
  Card,
  Container,
  Grid,
  makeStyles,
  Tab,
  Tabs,
  Typography,
} from '@material-ui/core'
import { filterFor } from '../helpers'
import { Fragment, useState } from 'react'
import { Link, Route, Switch, useRouteMatch } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import PropTypes from 'prop-types'
import React from 'react'
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

  const [currentTab, setCurrentTab] = useState('analysis')
  const handleTabChange = (event, newValue) => {
    console.warn(newValue)
    setCurrentTab(newValue)
  }

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
        currentPage={currPage}
      />
      <Container xl>
        <Typography variant="h4" gutterBottom className={classes.title}>
          Payload Stream
        </Typography>

        <Typography variant="h5" gutterBottom className={classes.title}>
          {arch} {stream}
        </Typography>

        <Grid
          container
          justifyContent="center"
          width="60%"
          style={{ margin: 20 }}
        >
          <Tabs value={currentTab} onChange={handleTabChange}>
            <Tab label="Analysis" value="analysis" component={Link} to={url} />
            <Tab
              label="Payloads"
              value="payloads"
              component={Link}
              to={url + '/payloads'}
            />
          </Tabs>
        </Grid>

        <Switch>
          <Route path={path + '/payloads'}>
            <Card
              elevation={5}
              style={{ margin: 20, padding: 20, height: '100%' }}
            >
              <Typography variant="h6" gutterBottom className={classes.title}>
                Another Tab
              </Typography>
            </Card>
          </Route>

          <Route path={path + '/'}>
            <Card
              elevation={5}
              style={{ margin: 20, padding: 20, height: '100%' }}
            >
              <ReleaseStreamAnalysis
                release={props.release}
                stream={props.stream}
                arch={props.arch}
              />
            </Card>
          </Route>
        </Switch>
      </Container>
    </Fragment>
  )
}

ReleaseStreamDetails.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}
