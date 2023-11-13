import './Install.css'
import { Alert, TabContext } from '@mui/lab'
import { BOOKMARKS } from '../constants'
import { Grid, Paper, Tab, Tabs, Typography } from '@mui/material'
import { Link, Redirect, Route, Switch, useRouteMatch } from 'react-router-dom'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestByVariantTable from '../tests/TestByVariantTable'
import TestTable from '../tests/TestTable'
import TopLevelIndicators from './InstallTopLevelIndicators'

export default function Install(props) {
  const { path, url } = useRouteMatch()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [health, setHealth] = React.useState({})

  const fetchData = () => {
    Promise.all([
      fetch(
        process.env.REACT_APP_API_URL + '/api/install?release=' + props.release
      ),
      fetch(
        process.env.REACT_APP_API_URL + '/api/health?release=' + props.release
      ),
    ])
      .then(([install, health]) => {
        if (install.status !== 200) {
          throw new Error('server returned ' + install.status)
        }

        if (health.status !== 200) {
          throw new Error('server returned ' + health.status)
        }
        return Promise.all([install.json(), health.json()])
      })
      .then(([install, health]) => {
        setData(install)
        setHealth(health)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve install for release ' +
            props.release +
            ', ' +
            error
        )
      })
  }

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Install health`
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">Failed to load data, {fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Install" />
      <Route
        path="/"
        render={({ location }) => (
          <TabContext value={path}>
            <Typography align="center" variant="h4">
              Install health for {props.release}
            </Typography>
            <Grid container spacing={2} alignItems="stretch">
              <TopLevelIndicators
                release={props.release}
                indicators={health.indicators}
              />
              <Grid
                container
                justifyContent="center"
                size="xl"
                className="view"
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
                      label="Install rates by operators"
                      value="operators"
                      component={Link}
                      to={url + '/operators'}
                    />
                  </Tabs>
                </Paper>
              </Grid>
              <Switch>
                <Route path={path + '/operators'}>
                  <TestByVariantTable
                    release={props.release}
                    colorScale={[90, 100]}
                    data={data}
                    excludedVariants={[
                      'upgrade-minor',
                      'aggregated',
                      'never-stable',
                    ]}
                  />
                </Route>
                <Redirect from="/" to={url + '/operators'} />
              </Switch>
            </Grid>
          </TabContext>
        )}
      />
    </Fragment>
  )
}

Install.propTypes = {
  release: PropTypes.string,
}
