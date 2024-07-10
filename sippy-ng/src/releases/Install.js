import './Install.css'
import { Grid, Typography } from '@mui/material'
import { Redirect, Route, Switch, useRouteMatch } from 'react-router-dom'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestByVariantTable from '../tests/TestByVariantTable'
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
      <Grid>
        <Typography variant="h4" style={{ margin: 10 }} align="center">
          Install health for {props.release}
        </Typography>
      </Grid>
      <Route
        path="/"
        render={({ location }) => (
          <Fragment>
            <Grid container justifyContent="center" spacing={3}>
              <TopLevelIndicators
                release={props.release}
                indicators={health.indicators}
              />
            </Grid>

            <Grid>
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
          </Fragment>
        )}
      />
    </Fragment>
  )
}

Install.propTypes = {
  release: PropTypes.string,
}
