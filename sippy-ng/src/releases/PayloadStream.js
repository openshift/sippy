import { filterFor } from '../helpers'
import { Grid, Paper, Tab, Tabs, Typography } from '@mui/material'
import { Link, Redirect, Route, Switch, useRouteMatch } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { StringParam, useQueryParam } from 'use-query-params'
import { TabContext } from '@mui/lab'
import PayloadCalendar from './PayloadCalendar'
import PayloadStreamOverview from './PayloadStreamOverview'
import PayloadStreamTestFailures from './PayloadStreamTestFailures'
import PropTypes from 'prop-types'
import React, { Fragment, useState } from 'react'
import ReleasePayloadTable from './ReleasePayloadTable'
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

export default function PayloadStream(props) {
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
            <Fragment>
              <Typography variant="h4" gutterBottom className={classes.title}>
                {arch} {stream} {props.release} Payload Stream
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
                      label="Overview"
                      value="overview"
                      component={Link}
                      to={url + '/overview'}
                    />
                    <Tab
                      label="Calendar"
                      value="calendar"
                      component={Link}
                      to={url + '/calendar'}
                    />
                    <Tab
                      label="Payloads"
                      value="payloads"
                      component={Link}
                      to={url + '/payloads'}
                    />
                    <Tab
                      label="Test Failures"
                      value="testfailures"
                      component={Link}
                      to={url + '/testfailures'}
                    />
                  </Tabs>
                </Paper>
              </Grid>
              <Switch>
                <Route path={path + '/overview'}>
                  <PayloadStreamOverview
                    release={props.release}
                    stream={props.stream}
                    arch={props.arch}
                  />
                </Route>
                <Route path={path + '/calendar'}>
                  <PayloadCalendar
                    release={props.release}
                    arch={props.arch}
                    stream={props.stream}
                  />
                </Route>
                <Route path={path + '/testfailures'}>
                  <PayloadStreamTestFailures
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
                <Redirect from="/" to={url + '/overview'} />
              </Switch>
            </Fragment>
          </TabContext>
        )}
      />
    </Fragment>
  )
}

PayloadStream.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}
