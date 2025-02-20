import { Box, Paper, Tab, Tabs, Typography } from '@mui/material'
import { TabContext } from '@mui/lab'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

import { BOOKMARKS } from '../constants'
import {
  Link,
  Route,
  Switch,
  useLocation,
  useRouteMatch,
} from 'react-router-dom'
import { pathForAPIWithFilter, withSort } from '../helpers'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestTable from './TestTable'

/**
 * Tests is the landing page for tests, with tabs for all tests,
 * and test results by variant.
 */
export default function Tests(props) {
  const { path, url } = useRouteMatch()
  const search = useLocation().search

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Tests`
  }, [])

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Tests" />

      <Route
        path="/"
        render={({ location }) => (
          <TabContext value={path}>
            <Typography align="center" variant="h4">
              Tests for {props.release}
            </Typography>
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
              }}
            >
              <Paper
                sx={{
                  margin: 2,
                  border: 1,
                  borderColor: 'divider',
                  display: 'inline-block',
                }}
              >
                <Tabs
                  value={location.pathname.substring(
                    location.pathname.lastIndexOf('/') + 1
                  )}
                  indicatorColor="primary"
                  textColor="primary"
                >
                  <Tab
                    label="All tests"
                    value={props.release}
                    component={Link}
                    sx={{ padding: '6px 12px !important' }}
                    to={url + search}
                  />
                  <Tab
                    label="Tests by variant"
                    value="details"
                    sx={{ padding: '6px 12px !important' }}
                    component={Link}
                    to={url + '/details' + search}
                  />
                </Tabs>
              </Paper>
            </Box>
            <Switch>
              <Route path={path + '/details'}>
                <TestTable release={props.release} collapse={false} />
              </Route>
              <Route exact path={path}>
                <TestTable release={props.release} />
              </Route>
            </Switch>
          </TabContext>
        )}
      />
    </Fragment>
  )
}

Tests.propTypes = {
  release: PropTypes.string,
}
