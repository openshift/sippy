import {
  Box,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Container,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from '@mui/material'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/material/Alert'
import HelpOutlineIcon from '@mui/icons-material/HelpOutline'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestTable from './TestTable'

export default function FeatureGateDetail(props) {
  const { release, featureGate } = props

  const [gate, setGate] = React.useState(null)
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')
  const [activeTab, setActiveTab] = React.useState(0)

  useEffect(() => {
    document.title = `Sippy > ${release} > Feature Gates > ${featureGate}`
    setLoaded(false)
    setFetchError('')

    const filterParam = safeEncodeURIComponent(
      JSON.stringify({
        items: [
          {
            columnField: 'feature_gate',
            operatorValue: 'equals',
            value: featureGate,
          },
        ],
      })
    )

    fetch(
      import.meta.env.VITE_API_URL +
        '/api/feature_gates?release=' +
        release +
        '&filter=' +
        filterParam
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        if (json && json.length > 0) {
          setGate(json[0])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve feature gate: ' + error)
        setLoaded(true)
      })
  }, [release, featureGate])

  const tabAutoSelected = React.useRef(false)

  // <= 1 because the API always returns an implicit "Overall" row
  const handleAnnotationDataLoaded = React.useCallback((count) => {
    if (!tabAutoSelected.current && count <= 1) {
      setActiveTab(1)
      tabAutoSelected.current = true
    }
  }, [])

  const annotationFilter = {
    items: [
      {
        columnField: 'name',
        operatorValue: 'contains',
        value: `FeatureGate:${featureGate}]`,
      },
    ],
  }

  // Installer gates currently run a broad conformance suite where full passes
  // aren't required, so "install should succeed" is the meaningful signal.
  // Switch to "openshift-tests should work" once installer jobs run a minimal
  // conformance suite.
  const capabilityTestName = featureGate.includes('Install')
    ? 'install should succeed'
    : 'openshift-tests should work'

  const capabilityFilter = {
    items: [
      {
        columnField: 'name',
        operatorValue: 'contains',
        value: capabilityTestName,
      },
      {
        columnField: 'variants',
        operatorValue: 'has entry containing',
        value: `Capability:${featureGate}`,
      },
    ],
    linkOperator: 'and',
  }

  if (fetchError) {
    return (
      <Container size="xl">
        <Alert severity="error">{fetchError}</Alert>
      </Container>
    )
  }

  if (!isLoaded) {
    return (
      <Container
        size="xl"
        sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}
      >
        <CircularProgress />
      </Container>
    )
  }

  if (!gate) {
    return (
      <Container size="xl">
        <Alert severity="warning">
          Feature gate &quot;{featureGate}&quot; not found in release {release}.
        </Alert>
      </Container>
    )
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={release}
        currentPage={featureGate}
        previousPage={
          <Link to={`/feature_gates/${release}`}>Feature Gates</Link>
        }
      />
      <Container size="xl">
        <Typography align="center" variant="h4" sx={{ mb: 2 }}>
          {featureGate}
        </Typography>

        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="body1" sx={{ mb: 1 }}>
              <strong>Release:</strong> {gate.release}
            </Typography>
            <Typography variant="body1" sx={{ mb: 1 }}>
              <strong>First Seen In:</strong> {gate.first_seen_in}
            </Typography>
            <Typography variant="body1" sx={{ mb: 1 }}>
              <strong>Total Test Count:</strong> {gate.unique_test_count}
              <Tooltip title="Unique test names. The tables below may show more rows due to per-variant breakdowns.">
                <HelpOutlineIcon
                  fontSize="small"
                  sx={{
                    ml: 0.5,
                    verticalAlign: 'middle',
                    color: 'text.secondary',
                  }}
                />
              </Tooltip>
            </Typography>
            <Typography variant="body1" component="div">
              <strong>Enabled:</strong>{' '}
              {gate.enabled &&
                gate.enabled.map((e) => (
                  <Chip key={e} label={e} size="small" sx={{ mr: 0.5 }} />
                ))}
            </Typography>
          </CardContent>
        </Card>

        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs
            value={activeTab}
            onChange={(e, v) => setActiveTab(v)}
            aria-label="feature gate test sections"
          >
            <Tab label="Tests by Annotation" />
            <Tab label="Tests by Capability" />
          </Tabs>
        </Box>

        <Box sx={{ display: activeTab === 0 ? 'block' : 'none' }}>
          <TestTable
            key={'fg-annotation-' + featureGate}
            release={release}
            collapse={false}
            filterModel={annotationFilter}
            onDataLoaded={handleAnnotationDataLoaded}
          />
        </Box>

        <Box sx={{ display: activeTab === 1 ? 'block' : 'none' }}>
          <TestTable
            key={'fg-capability-' + featureGate}
            release={release}
            collapse={false}
            filterModel={capabilityFilter}
          />
        </Box>
      </Container>
    </Fragment>
  )
}

FeatureGateDetail.propTypes = {
  release: PropTypes.string.isRequired,
  featureGate: PropTypes.string.isRequired,
}
