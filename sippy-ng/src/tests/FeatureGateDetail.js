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
import { SafeJSONParam } from '../helpers'
import { useQueryParam } from 'use-query-params'
import Alert from '@mui/material/Alert'
import FeatureGatePromotionTab from './FeatureGatePromotionTab'
import HelpOutlineIcon from '@mui/icons-material/HelpOutline'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect, useMemo } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestTable from './TestTable'

export default function FeatureGateDetail(props) {
  const { release, featureGate } = props

  const [gate, setGate] = React.useState(null)
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')
  const [activeTab, setActiveTabRaw] = React.useState(0)
  const [, setFiltersParam] = useQueryParam('filters', SafeJSONParam)

  const setActiveTab = React.useCallback(
    (tab) => {
      setFiltersParam(undefined)
      setActiveTabRaw(tab)
    },
    [setFiltersParam]
  )

  useEffect(() => {
    document.title = `Sippy > ${release} > Feature Gates > ${featureGate}`
    setLoaded(false)
    setFetchError('')

    fetch(
      `${process.env.REACT_APP_API_URL}/api/feature_gates/${encodeURIComponent(
        featureGate
      )}?release=${encodeURIComponent(release)}`
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setGate(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve feature gate: ' + error)
        setLoaded(true)
      })
  }, [release, featureGate])

  const annotationFilter = {
    items: [
      {
        columnField: 'name',
        operatorValue: 'contains',
        value: `FeatureGate:${featureGate}]`,
      },
    ],
  }

  const installFilter = {
    items: [
      {
        columnField: 'name',
        operatorValue: 'contains',
        value: 'install should succeed',
      },
      {
        columnField: 'variants',
        operatorValue: 'has entry containing',
        value: `Capability:${featureGate}`,
      },
    ],
    linkOperator: 'and',
  }

  const jobTestsFilter = {
    items: [
      {
        columnField: 'variants',
        not: true,
        operatorValue: 'has entry',
        value: 'never-stable',
      },
      {
        columnField: 'variants',
        not: true,
        operatorValue: 'has entry',
        value: 'aggregated',
      },
      {
        columnField: 'variants',
        operatorValue: 'has entry',
        value: `Capability:${featureGate}`,
      },
      {
        columnField: 'current_working_percentage',
        operatorValue: '<',
        value: '92',
      },
      {
        columnField: 'current_runs',
        operatorValue: '>=',
        value: '0',
      },
      {
        columnField: 'name',
        not: true,
        operatorValue: 'contains',
        value: 'install should succeed',
      },
      {
        columnField: 'name',
        not: true,
        operatorValue: 'contains',
        value: 'openshift-tests should work',
      },
      {
        columnField: 'name',
        not: true,
        operatorValue: 'contains',
        value: 'infrastructure should work',
      },
    ],
    linkOperator: 'and',
  }

  const tabs = useMemo(() => {
    const t = [
      { key: 'promotion', label: 'Promotion Readiness' },
      { key: 'gate_tests', label: 'Gate Tests' },
    ]
    if (gate?.links?.install_tests) {
      t.push({ key: 'install_tests', label: 'Install Tests' })
    }
    if (gate?.links?.gate_job_tests) {
      t.push({ key: 'gate_job_tests', label: 'Gate Job Tests' })
    }
    return t
  }, [gate])

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
            <Typography variant="body1" component="div" sx={{ mt: 1 }}>
              <strong>Matching Jobs:</strong>{' '}
              {gate.matching_jobs && gate.matching_jobs.length > 0
                ? gate.matching_jobs.map((job) => (
                    <Chip
                      key={job}
                      label={job}
                      size="small"
                      sx={{ mr: 0.5, mb: 0.5 }}
                    />
                  ))
                : 'None'}
            </Typography>
          </CardContent>
        </Card>

        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs
            value={activeTab}
            onChange={(e, v) => setActiveTab(v)}
            aria-label="feature gate test sections"
          >
            {tabs.map((t) => (
              <Tab key={t.key} label={t.label} />
            ))}
          </Tabs>
        </Box>

        {tabs[activeTab]?.key === 'promotion' && (
          <FeatureGatePromotionTab
            key={'fg-promotion-' + featureGate}
            release={release}
            featureGate={featureGate}
            onCellClick={() => {
              setActiveTab(tabs.findIndex((tab) => tab.key === 'gate_tests'))
            }}
          />
        )}
        {tabs[activeTab]?.key === 'gate_tests' && (
          <TestTable
            key={'fg-annotation-' + featureGate}
            release={release}
            collapse={false}
            filterModel={annotationFilter}
          />
        )}
        {tabs[activeTab]?.key === 'install_tests' && (
          <TestTable
            key={'fg-install-' + featureGate}
            release={release}
            collapse={false}
            filterModel={installFilter}
          />
        )}
        {tabs[activeTab]?.key === 'gate_job_tests' && (
          <TestTable
            key={'fg-jobtests-' + featureGate}
            release={release}
            collapse={false}
            filterModel={jobTestsFilter}
          />
        )}
      </Container>
    </Fragment>
  )
}

FeatureGateDetail.propTypes = {
  release: PropTypes.string.isRequired,
  featureGate: PropTypes.string.isRequired,
}
