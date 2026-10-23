import {
  Box,
  Card,
  CircularProgress,
  Container,
  Tooltip,
  Typography,
} from '@mui/material'
import { CardHeaderRow, StyledInfoIcon } from './releaseStyledComponents'
import { Link } from 'react-router'
import { ReleasesContext } from '../App'
import { styled } from '@mui/material/styles'
import Alert from '@mui/material/Alert'
import Grid from '@mui/material/Grid'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import RecentTestFailures from './RecentTestFailures'
import ReleaseKeyDates from './ReleaseKeyDates'
import ReleasePayloadAcceptance from './ReleasePayloadAcceptance'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TopLevelIndicators from './TopLevelIndicators'

const FullScreenLoadingBox = styled(Box)({
  display: 'flex',
  justifyContent: 'center',
  alignItems: 'center',
  minHeight: '60vh',
})

const PageWrapper = styled(Box)(({ theme }) => ({
  paddingTop: theme.spacing(3),
  paddingBottom: theme.spacing(3),
  paddingLeft: theme.spacing(2.5),
  paddingRight: theme.spacing(2.5),
}))

const TitleCenterBox = styled(Box)({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  height: '100%',
})

export default function ReleaseOverview(props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const releases = React.useContext(ReleasesContext)

  const fetchData = () => {
    fetch(import.meta.env.VITE_API_URL + '/api/health?release=' + props.release)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setData(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve release ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Health Summary`
    fetchData()
  }, [])

  if (fetchError !== '') {
    return (
      <Container
        maxWidth="lg"
        sx={(theme) => ({ marginTop: theme.spacing(4) })}
      >
        <Alert severity="error">{fetchError}</Alert>
      </Container>
    )
  }

  if (!isLoaded) {
    return (
      <FullScreenLoadingBox>
        <Box sx={{ textAlign: 'center' }}>
          <CircularProgress size={48} />
          <Typography
            variant="body1"
            color="text.secondary"
            sx={(theme) => ({ marginTop: theme.spacing(2) })}
          >
            Loading release overview...
          </Typography>
        </Box>
      </FullScreenLoadingBox>
    )
  }

  const warnings = []
  if (data.warnings && data.warnings.length > 0) {
    data.warnings.forEach((warning, index) => {
      warnings.push(
        <Grid item xs={12} key={'sippy-warning-' + index}>
          <Alert severity="warning">
            <div dangerouslySetInnerHTML={{ __html: warning }}></div>
          </Alert>
        </Grid>
      )
    })
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} />
      <PageWrapper>
        <Container maxWidth="lg">
          <Grid
            container
            spacing={3}
            alignItems="stretch"
            sx={(theme) => ({ marginBottom: theme.spacing(3) })}
          >
            <Grid item xs={12} md={8}>
              <TitleCenterBox>
                <Typography variant="h4" sx={{ fontWeight: 600 }}>
                  {props.release} Overview
                </Typography>
              </TitleCenterBox>
            </Grid>
            <ReleaseKeyDates release={props.release} releases={releases} />
          </Grid>

          <Grid container spacing={3} alignItems="stretch">
            {warnings}
            <TopLevelIndicators
              release={props.release}
              indicators={data.indicators}
              releases={releases}
            />

            <Grid item xs={12}>
              <RecentTestFailures release={props.release} />
            </Grid>

            {releases?.release_attrs?.[props.release]?.capabilities
              ?.payloadTags && (
              <Grid item xs={12}>
                <Card
                  elevation={5}
                  sx={(theme) => ({ padding: theme.spacing(2.5) })}
                >
                  <CardHeaderRow>
                    <Typography
                      variant="h6"
                      component={Link}
                      to={`/release/${props.release}/tags`}
                      sx={{ textDecoration: 'none', color: 'inherit' }}
                    >
                      Payload Acceptance
                    </Typography>
                    <Tooltip
                      title={
                        'These cards show the last accepted payload for each architecture/stream combination.'
                      }
                    >
                      <StyledInfoIcon />
                    </Tooltip>
                  </CardHeaderRow>

                  <Grid
                    container
                    spacing={3}
                    justifyContent="flex-start"
                    alignItems="stretch"
                  >
                    <ReleasePayloadAcceptance release={props.release} />
                  </Grid>
                </Card>
              </Grid>
            )}
          </Grid>
        </Container>
      </PageWrapper>
    </Fragment>
  )
}

ReleaseOverview.propTypes = {
  release: PropTypes.string.isRequired,
}
