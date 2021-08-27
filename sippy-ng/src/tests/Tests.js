import {
  Container,
  Grid,
  Paper,
  Tab,
  Tabs,
  Typography,
} from '@material-ui/core'
import { TabContext } from '@material-ui/lab'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

import { Link, Route, Switch, useRouteMatch } from 'react-router-dom'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestDetails from './TestsDetail'
import TestTable from './TestTable'

/**
 * Tests is the landing page for tests, with tabs for all tests,
 * and test results by variant.
 */
export default function Tests(props) {
  const { path, url } = useRouteMatch()

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
            <Grid
              container
              justifyContent="center"
              width="60%"
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
                    label="All tests"
                    value={props.release}
                    component={Link}
                    to={url}
                  />
                  <Tab
                    label="Tests by variant"
                    value="details"
                    component={Link}
                    to={url + '/details'}
                  />
                </Tabs>
              </Paper>
            </Grid>
            <Switch>
              <Route path={path + '/details'}>
                <TestDetails release={props.release} />
              </Route>

              <Route exact path={path}>
                <Container size="xl">
                  <TestTable release={props.release} />
                </Container>
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
