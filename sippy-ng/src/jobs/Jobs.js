import PropTypes from 'prop-types'
import { Container, Grid, Paper, Tab, Tabs, Typography } from '@material-ui/core'
import { TabContext } from '@material-ui/lab'
import React, { Fragment, useEffect } from 'react'

import { Link, Route, Switch, useRouteMatch } from 'react-router-dom'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import JobsDetail from './JobsDetail'
import JobTable from './JobTable'
import Variants from './Variants'
import JobRunsTable from './JobRunsTable'

/**
 * Jobs is the landing page for jobs with tabs for all jobs, variants,
 * and job runs.
 */
export default function Jobs (props) {
  const { path, url } = useRouteMatch()

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Jobs`
  }, [])

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Jobs" />
      <Route
        path="/"
        render={({ location }) => (
          <TabContext value={path}>
            <Typography align="center" variant="h4">
              Job health for {props.release}
            </Typography>
            <Grid container justifyContent="center" width="60%" style={{ margin: 20 }}>
              <Paper>
                <Tabs
                  value={location.pathname.substring(location.pathname.lastIndexOf('/') + 1)}
                  indicatorColor="primary"
                  textColor="primary"
                >
                  <Tab label="All jobs" value={props.release} component={Link} to={url} />
                  <Tab label="Jobs by variant" value="variant" component={Link} to={url + '/variant'} />
                  <Tab label="All job runs" value="runs" component={Link} to={url + '/runs'} />
                  <Tab label="Job run summary" value="detail" component={Link} to={url + '/detail'} />
                </Tabs>
              </Paper>
            </Grid>
            <Container size="xl">
              <Switch>
                <Route path={path + '/variant'}>
                  <Variants release={props.release} />
                </Route>

                <Route path={path + '/detail'}>
                  <JobsDetail release={props.release} />
                </Route>

                <Route path={path + '/runs'}>
                  <JobRunsTable release={props.release} />
                </Route>

                <Route exact path={path}>
                  <JobTable release={props.release} />
                </Route>
              </Switch>
            </Container>
          </TabContext>
        )}
      />
    </Fragment>
  )
}

Jobs.propTypes = {
  release: PropTypes.string.isRequired
}
