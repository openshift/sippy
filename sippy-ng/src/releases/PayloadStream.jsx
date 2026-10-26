import { filterFor } from '../helpers'
import { Grid, Paper, Tab, Tabs, Typography } from '@mui/material'
import { Link, Navigate, Route, Routes, useLocation } from 'react-router'
import { StringParam, useQueryParam } from 'use-query-params'

import PayloadCalendar from './PayloadCalendar'
import PayloadStreamOverview from './PayloadStreamOverview'
import PayloadStreamTestFailures from './PayloadStreamTestFailures'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import ReleasePayloadTable from './ReleasePayloadTable'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

export default function PayloadStream(props) {
  const location = useLocation()
  const basePath = `/release/${props.release}/streams/${props.arch}/${props.stream}`

  const [_release = props.release, _setRelease] = useQueryParam(
    'release',
    StringParam
  )
  const [arch = props.arch, _setArch] = useQueryParam('arch', StringParam)
  const [stream = props.stream, _setStream] = useQueryParam(
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
      <Typography variant="h4" gutterBottom sx={{ textAlign: 'center' }}>
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
            <Typography variant="h4" gutterBottom sx={{ textAlign: 'center' }}>
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
  )
}

PayloadStream.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}
