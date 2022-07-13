import {
  Card,
  Container,
  Grid,
  makeStyles,
  Paper,
  Tab,
  Tabs,
  Typography,
} from '@material-ui/core'
import { filterFor, not, SafeJSONParam } from '../helpers'
import { Fragment, useState } from 'react'
import { Link, Redirect, Route, Switch, useRouteMatch } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import { TabContext, TabPanel } from '@material-ui/lab'
import PropTypes from 'prop-types'
import React from 'react'
import ReleasePayloadTable from './ReleasePayloadTable'
import ReleaseStreamAnalysis from './ReleaseStreamAnalysis'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestByVariantTable from '../tests/TestByVariantTable'

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

  const [currentTab, setCurrentTab] = useState(0)
  function handleTabChange(event, newValue) {
    console.warn('Setting new value ' + newValue)
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

  let payloadsFilterModel = {
    items: [
      filterFor('architecture', 'equals', props.arch),
      filterFor('stream', 'equals', props.stream),
    ],
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={
          <Link to={`/release/${props.release}/streams`}>Payload Streams</Link>
        }
        currentPage={currPage}
      />
      <Route
        path="/"
        render={({ location }) => (
          <TabContext value={path}>
            <Container xl>
              <Typography variant="h4" gutterBottom className={classes.title}>
                {arch} {stream} Payload Stream
              </Typography>

              <Grid
                container
                justifyContent="center"
                width="100%"
                style={{ margin: 20 }}
              >
                <Paper>
                  <Tabs
                    value={location.pathname.substring(
                      location.pathname.lastIndexOf('/') + 1
                    )}
                    indicatorColor="primary"
                    textColor="primary"
                  >
                    <Tab
                      label="Payloads"
                      value="payloads"
                      component={Link}
                      to={url + '/payloads'}
                    />
                    <Tab
                      label="Test Failures"
                      value="failures"
                      component={Link}
                      to={url + '/testfailures'}
                    />
                  </Tabs>
                </Paper>
              </Grid>
              <Switch>
                <Route path={path + '/testfailures'}>
                  <ReleaseStreamAnalysis
                    release={props.release}
                    stream={props.stream}
                    arch={props.arch}
                  />
                </Route>
                <Route path={path + '/payloads'}>
                  <Typography
                    variant="h4"
                    gutterBottom
                    className={classes.title}
                  >
                    <ReleasePayloadTable
                      release={props.release}
                      filterModel={payloadsFilterModel}
                    />
                  </Typography>
                </Route>
                <Redirect from="/" to={url + '/payloads'} />
              </Switch>
            </Container>
          </TabContext>
        )}
      />
    </Fragment>
  )
}

ReleaseStreamDetails.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}
