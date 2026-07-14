import {
  Chip,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material'
import { Link } from 'react-router-dom'
import { relativeTime, safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { useEffect, useMemo } from 'react'

export default function TestRegressionsTable({
  release,
  testName,
  filterModel,
}) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [regressions, setRegressions] = React.useState([])
  const [fetchError, setFetchError] = React.useState('')

  useEffect(() => {
    if (!testName || !release) return

    const url = `${
      process.env.REACT_APP_API_URL
    }/api/component_readiness/regressions?release=${safeEncodeURIComponent(
      release
    )}&test_name=${safeEncodeURIComponent(testName)}`

    fetch(url)
      .then((res) => {
        if (res.status !== 200) {
          throw new Error('server returned ' + res.status)
        }
        return res.json()
      })
      .then((data) => {
        setRegressions(data || [])
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve regression data: ' + error)
        setLoaded(true)
      })
  }, [release, testName])

  const variantFilters = useMemo(() => {
    if (!filterModel || !filterModel.items) return []
    return filterModel.items.filter((f) => f.columnField === 'variants')
  }, [filterModel])

  const filteredRegressions = useMemo(() => {
    let filtered = regressions.filter((r) => !r.closed || !r.closed.Valid)

    if (variantFilters.length === 0) return filtered

    return filtered.filter((regression) => {
      const variantValues = (regression.variants || []).map((v) => {
        const parts = v.split(':')
        return parts.length > 1 ? parts.slice(1).join(':') : v
      })

      return variantFilters.every((filter) => {
        const hasMatch = variantValues.some(
          (v) => v.toLowerCase() === filter.value.toLowerCase()
        )
        if (filter.not) {
          return !hasMatch
        }
        return hasMatch
      })
    })
  }, [regressions, variantFilters])

  if (!isLoaded) {
    return <p>Loading...</p>
  }
  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }
  if (filteredRegressions.length === 0) {
    return <Typography>No active regressions found</Typography>
  }

  return (
    <TableContainer component={Paper} style={{ marginTop: 20 }}>
      <Table size="small" aria-label="regressions-table">
        <TableHead>
          <TableRow>
            <TableCell>ID</TableCell>
            <TableCell>Variants</TableCell>
            <TableCell>Regressed Since</TableCell>
            <TableCell>Last Failure</TableCell>
            <TableCell>Triage</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {filteredRegressions.map((regression) => {
            const variantValues = (regression.variants || [])
              .map((v) => {
                const parts = v.split(':')
                return parts.length > 1 ? parts.slice(1).join(':') : v
              })
              .filter((val) => !['default', 'none', 'unknown'].includes(val))

            const tooltipLines = (regression.variants || []).sort().join('\n')

            const triaged = regression.triages && regression.triages.length > 0

            return (
              <TableRow key={regression.id}>
                <TableCell>
                  <Link
                    to={`/sippy-ng/component_readiness/regressions/${regression.id}`}
                  >
                    {regression.id}
                  </Link>
                </TableCell>
                <TableCell>
                  <Tooltip
                    title={
                      <span style={{ whiteSpace: 'pre-line' }}>
                        {tooltipLines}
                      </span>
                    }
                  >
                    <span>{variantValues.join(', ')}</span>
                  </Tooltip>
                </TableCell>
                <TableCell>
                  {regression.opened
                    ? relativeTime(new Date(regression.opened), new Date())
                    : ''}
                </TableCell>
                <TableCell>
                  {regression.last_failure && regression.last_failure.Valid
                    ? relativeTime(
                        new Date(regression.last_failure.Time),
                        new Date()
                      )
                    : ''}
                </TableCell>
                <TableCell>
                  {triaged ? (
                    regression.triages.map((t) => (
                      <Chip
                        key={t.id}
                        label={
                          t.url ? t.url.split('/').pop() : `Triage #${t.id}`
                        }
                        component={Link}
                        to={`/sippy-ng/component_readiness/triages/${t.id}`}
                        clickable
                        size="small"
                        color="primary"
                        variant="outlined"
                        sx={{ marginRight: 0.5 }}
                      />
                    ))
                  ) : (
                    <Chip label="Untriaged" size="small" color="warning" />
                  )}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

TestRegressionsTable.propTypes = {
  release: PropTypes.string.isRequired,
  testName: PropTypes.string.isRequired,
  filterModel: PropTypes.object,
}
