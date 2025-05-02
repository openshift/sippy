import { Box, Container, Paper, Tab, Tabs, Typography } from '@mui/material'
import { TabContext } from '@mui/lab'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

import { filterFor } from '../helpers'
import { Link, Route, Switch, useRouteMatch } from 'react-router-dom'
import JobRunsTable from './JobRunsTable'
import JobTable from './JobTable'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

/**
 * Jobs is the landing page for jobs with tabs for all jobs, variants,
 * and job runs.
 */
export default function Jobs(props) {
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
                    label="All jobs"
                    value={props.release}
                    component={Link}
                    to={url}
                    sx={{ padding: '6px 12px !important' }}
                  />
                  <Tab
                    label="Permafailing"
                    value="permafailing"
                    component={Link}
                    to={url + '/permafailing'}
                    sx={{ padding: '6px 12px !important' }}
                  />
                  <Tab
                    label="All job runs"
                    value="runs"
                    component={Link}
                    to={url + '/runs'}
                    sx={{ padding: '6px 12px !important' }}
                  />
                </Tabs>
              </Paper>
            </Box>
            <Container size="xl">
              <Switch>
                <Route path={path + '/runs'}>
                  <JobRunsTable release={props.release} />
                </Route>
                <Route path={path + '/permafailing'}>
                  <JobTable
                    release={props.release}
                    filterModel={{
                      items: [
                        filterFor('current_pass_percentage', '=', '0'),
                        filterFor('previous_pass_percentage', '=', '0'),
                      ],
                    }}
                    sortField="last_pass"
                    sort="desc"
                    view="Last passing"
                    hideControls="true"
                  />
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
  release: PropTypes.string.isRequired,
}
