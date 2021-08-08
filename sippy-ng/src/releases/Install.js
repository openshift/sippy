import { Grid, Paper, Tab, Tabs, Typography } from '@material-ui/core'
import { Alert, TabContext } from '@material-ui/lab'
import React, { useEffect, Fragment } from 'react'
import PropTypes from 'prop-types'
import { Link, Redirect, Route, Switch, useRouteMatch } from 'react-router-dom'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestByVariantTable from '../tests/TestByVariantTable'
import TestTable from '../tests/TestTable'

export default function Install (props) {
  const { path, url } = useRouteMatch()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  const fetchData = () => {
    fetch(process.env.REACT_APP_API_URL + '/api/install?release=' + props.release)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then(json => {
        setData(json)
        setLoaded(true)
      }).catch(error => {
        setFetchError('Could not retrieve release ' + props.release + ', ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">Failed to load data, {fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  };

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
                        <Grid container justifyContent="center" width="60%" style={{ margin: 20 }}>
                            <Paper>
                                <Tabs
                                    value={location.pathname.substring(location.pathname.lastIndexOf('/') + 1)}
                                    indicatorColor="primary"
                                    textColor="primary"
                                >
                                    <Tab label="Install rates by operator" value="operators" component={Link} to={url + '/operators'} />
                                    <Tab label="Install related tests" value="tests" component={Link} to={url + '/tests'} />
                                </Tabs>
                            </Paper>
                        </Grid>
                        <Switch>
                            <Route path={path + '/operators'}>
                                <TestByVariantTable release={props.release} colorScale={[90, 100]} data={data} />
                            </Route>
                            <Route path={path + '/tests'}>
                                <TestTable release={props.release} filterBy={['install']} />
                            </Route>
                            <Redirect from="/" to={url + '/operators'} />
                        </Switch>
                    </TabContext>
                )}
            />
        </Fragment>
  )
}

Install.propTypes = {
  release: PropTypes.string
}
