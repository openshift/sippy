import {
  Box,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Container,
  Tab,
  Tabs,
  Typography,
} from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { filterFor, multiple, safeEncodeURIComponent } from '../helpers'
import { Link } from 'react-router-dom'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

function TestResultsSection({ title, apiUrl, release, extraFilters = [] }) {
  const [rows, setRows] = React.useState([])
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')
  const [sortModel, setSortModel] = React.useState([
    { field: 'current_pass_percentage', sort: 'asc' },
  ])

  useEffect(() => {
    if (!apiUrl) {
      setRows([])
      setLoaded(true)
      return
    }
    setLoaded(false)
    setFetchError('')

    fetch(apiUrl)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setRows(json || [])
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve ' + title + ': ' + error)
        setLoaded(true)
      })
  }, [apiUrl, title])

  const columns = [
    {
      field: 'name',
      headerName: 'Test Name',
      flex: 4,
      renderCell: (params) => {
        const path =
          `/tests/${release}/details?` +
          multiple(filterFor('name', 'equals', params.value), ...extraFilters)
        return <Link to={path}>{params.value}</Link>
      },
    },
    {
      field: 'current_successes',
      headerName: 'Current Successes',
      type: 'number',
      flex: 1,
    },
    {
      field: 'current_failures',
      headerName: 'Current Failures',
      type: 'number',
      flex: 1,
    },
    {
      field: 'current_flakes',
      headerName: 'Current Flakes',
      type: 'number',
      flex: 1,
    },
    {
      field: 'current_pass_percentage',
      headerName: 'Current Pass %',
      type: 'number',
      flex: 1,
      renderCell: (params) =>
        params.value != null ? params.value.toFixed(2) + '%' : '',
    },
    {
      field: 'current_runs',
      headerName: 'Current Runs',
      type: 'number',
      flex: 1,
    },
  ]

  if (fetchError) {
    return <Alert severity="error">{fetchError}</Alert>
  }

  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>
        {title} ({isLoaded ? rows.length : '...'} tests)
      </Typography>
      <DataGrid
        loading={!isLoaded}
        rows={rows}
        columns={columns}
        getRowId={(row) => row.name}
        getRowHeight={() => 'auto'}
        autoHeight={true}
        rowsPerPageOptions={[10, 25, 50]}
        pageSize={25}
        sortModel={sortModel}
        onSortModelChange={setSortModel}
        disableSelectionOnClick
      />
    </Box>
  )
}

TestResultsSection.propTypes = {
  title: PropTypes.string.isRequired,
  apiUrl: PropTypes.string,
  release: PropTypes.string.isRequired,
  extraFilters: PropTypes.array,
}

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
      process.env.REACT_APP_API_URL +
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

        {activeTab === 0 && gate.links && (
          <TestResultsSection
            title="Tests tagged with FeatureGate annotation"
            apiUrl={gate.links.tests_by_annotation}
            release={release}
          />
        )}

        {activeTab === 1 && gate.links && (
          <TestResultsSection
            title="Tests matching capability variant"
            apiUrl={gate.links.tests_by_capability}
            release={release}
            extraFilters={[
              filterFor(
                'variants',
                'has entry containing',
                `Capability:${featureGate}`
              ),
            ]}
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
