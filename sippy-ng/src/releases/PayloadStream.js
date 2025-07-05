import { filterFor } from '../helpers'
import { Grid, Paper, Tab, Tabs, Typography } from '@mui/material'
import { Link, Navigate, Route, Routes, useLocation } from 'react-router-dom'
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
  const location = useLocation()
  const basePath = `/release/${props.release}/streams/${props.arch}/${props.stream}`

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
      <TabContext value={location.pathname}>
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
                  to={basePath + '/overview'}
                />
                <Tab
                  label="Calendar"
                  value="calendar"
                  component={Link}
                  to={basePath + '/calendar'}
                />
                <Tab
                  label="Payloads"
                  value="payloads"
                  component={Link}
                  to={basePath + '/payloads'}
                />
                <Tab
                  label="Test Failures"
                  value="testfailures"
                  component={Link}
                  to={basePath + '/testfailures'}
                />
              </Tabs>
            </Paper>
          </Grid>
          <Routes>
            <Route
              path="overview"
              element={
                <PayloadStreamOverview
                  release={props.release}
                  stream={props.stream}
                  arch={props.arch}
                />
              }
            />
            <Route
              path="calendar"
              element={
                <PayloadCalendar
                  release={props.release}
                  arch={props.arch}
                  stream={props.stream}
                />
              }
            />
            <Route
              path="testfailures"
              element={
                <PayloadStreamTestFailures
                  release={props.release}
                  stream={props.stream}
                  arch={props.arch}
                />
              }
            />
            <Route
              path="payloads"
              element={
                <Typography variant="h4" gutterBottom className={classes.title}>
                  <ReleasePayloadTable
                    release={props.release}
                    filterModel={payloadsFilterModel}
                  />
                </Typography>
              }
            />
            <Route
              path="/"
              element={<Navigate to={basePath + '/overview'} replace />}
            />
          </Routes>
        </Fragment>
      </TabContext>
    </Fragment>
  )
}

PayloadStream.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}
