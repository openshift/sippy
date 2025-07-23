import {
  Card,
  Container,
  Grid,
  Paper,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from '@mui/material'
import { filterFor, safeEncodeURIComponent } from '../helpers'
import { Fragment } from 'react'
import { Link, Route, Switch, useRouteMatch } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { StringParam, useQueryParam } from 'use-query-params'
import { WarningOutlined } from '@mui/icons-material'
import Alert from '@mui/material/Alert'
import PayloadTestFailures from './PayloadTestFailures'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'
import ReleasePayloadJobRuns from './ReleasePayloadJobRuns'
import ReleasePayloadPullRequests from './ReleasePayloadPullRequests'
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

export default function ReleasePayloadDetails(props) {
  const classes = useStyles()
  const { path, url } = useRouteMatch()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)

  const [tag, setTag] = React.useState({})

  const [releaseTag = props.releaseTag, setReleaseTag] = useQueryParam(
    'release_tag',
    StringParam
  )

  const fetchData = () => {
    const filter = safeEncodeURIComponent(
      JSON.stringify({
        items: [filterFor('release_tag', 'equals', releaseTag)],
      })
    )

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/releases/tags?filter=${filter}`
      ),
    ])
      .then(([tag]) => {
        if (tag.status !== 200) {
          throw new Error('server returned ' + tag.status)
        }

        return Promise.all([tag.json()])
      })
      .then(([tag]) => {
        if (tag.length === 0) {
          return <Typography variant="h5">Tag #{tag} not found.</Typography>
        }

        setTag(tag[0])
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve test failures ' + props.release + ', ' + error
        )
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
  }

  let statusAlert = (
    <Alert severity="success">This release payload was accepted.</Alert>
  )
  if (tag.phase === 'Rejected') {
    statusAlert = (
      <Alert severity="error">This release payload was rejected.</Alert>
    )
  } else if (tag.phase !== 'Accepted') {
    statusAlert = (
      <Alert severity="warning">
        Unexpected status {tag.phase} for this release payload.
      </Alert>
    )
  }

  let streamSuffix = `-${tag.architecture}`
  if (tag.architecture == 'amd64') {
    streamSuffix = ''
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={<Link to={`/release/${props.release}/tags`}>Tags</Link>}
        currentPage={releaseTag}
      />
      <Container maxWidth={'xl'}>
        <Typography variant="h4" gutterBottom className={classes.title}>
          {releaseTag}{' '}
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
                label="Job runs"
                value={releaseTag}
                component={Link}
                to={url}
              />
              <Tab
                label="Test Failures"
                value="failures"
                component={Link}
                to={url + '/testfailures'}
              />
              <Tab
                label="Pull requests"
                value="pull_requests"
                component={Link}
                to={url + '/pull_requests'}
              />
              <Tab
                label="Release controller"
                value="releaseController"
                href={`https://${tag.architecture}.ocp.releases.ci.openshift.org/releasestream/${tag.release}.0-0.${tag.stream}${streamSuffix}/release/${tag.release_tag}`}
              />
              <Tooltip title="Note: This payload may already be garbage collected on the release controller.">
                <WarningOutlined style={{ marginTop: 8, marginRight: 8 }} />
              </Tooltip>
            </Tabs>
          </Paper>
        </Grid>
        <Card elevation={5} style={{ margin: 20 }}>
          {statusAlert}
        </Card>
        <Switch>
          <Route path={path + '/pull_requests'}>
            <Card
              elevation={5}
              style={{ margin: 20, padding: 20, height: '100%' }}
            >
              <ReleasePayloadPullRequests
                filterModel={{
                  items: [filterFor('release_tag', 'equals', releaseTag)],
                }}
              />
            </Card>
          </Route>

          <Route path={path + '/testfailures'}>
            <Card
              elevation={5}
              style={{ margin: 20, padding: 20, height: '100%' }}
            >
              <PayloadTestFailures payload={props.releaseTag} />
            </Card>
          </Route>

          <Route path={path + '/'}>
            <Card
              elevation={5}
              style={{ margin: 20, padding: 20, height: '100%' }}
            >
              <ReleasePayloadJobRuns
                filterModel={{
                  items: [filterFor('release_tag', 'equals', releaseTag)],
                }}
              />
            </Card>
          </Route>
        </Switch>
      </Container>
    </Fragment>
  )
}

ReleasePayloadDetails.propTypes = {
  release: PropTypes.string,
  releaseTag: PropTypes.string.isRequired,
}
