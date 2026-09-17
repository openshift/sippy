import { Link } from 'react-router-dom'
import {
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
import {
  parseVariantName,
  relativeTime,
  safeEncodeURIComponent,
} from '../helpers'
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

    setLoaded(false)
    setFetchError('')

    const url = `${
      import.meta.env.VITE_API_URL
    }/api/component_readiness/regressions?release=${safeEncodeURIComponent(
      release
    )}&test=${safeEncodeURIComponent(testName)}`

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
      const variants = regression.variants || []

      return variantFilters.every((filter) => {
        const filterVal = filter.value.toLowerCase()
        const hasMatch = variants.some(
          (v) =>
            v.toLowerCase() === filterVal ||
            parseVariantName(v).name.toLowerCase() === filterVal
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
          </TableRow>
        </TableHead>
        <TableBody>
          {filteredRegressions.map((regression) => {
            const variantValues = (regression.variants || [])
              .map((v) => parseVariantName(v).name)
              .filter((val) => !['default', 'none', 'unknown'].includes(val))

            const tooltipLines = (regression.variants || []).sort().join('\n')

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
