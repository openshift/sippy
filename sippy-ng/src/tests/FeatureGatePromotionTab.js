import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  Collapse,
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
  Tooltip,
  Typography,
} from '@mui/material'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

const PASS_COLOR = '#d4edda'
const FAIL_COLOR = '#f8d7da'

function topologyDisplayName(topology) {
  if (topology === 'external') return 'hypershift'
  return topology
}

function variantColumnHeader(variant) {
  const parts = [
    topologyDisplayName(variant.topology),
    variant.cloud,
    variant.architecture,
  ]
  if (variant.network_stack) parts.push(variant.network_stack)
  if (variant.os) parts.push(`OS: ${variant.os}`)
  if (variant.optional) parts.push('(Optional)')
  return parts
}

export default function FeatureGatePromotionTab(props) {
  const { release, featureGate, onCellClick } = props

  const [data, setData] = React.useState(null)
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')
  const [showWarnings, setShowWarnings] = React.useState(false)
  const [showErrors, setShowErrors] = React.useState(false)
  const [orderBy, setOrderBy] = React.useState('test_name')
  const [order, setOrder] = React.useState('asc')

  useEffect(() => {
    setLoaded(false)
    setFetchError('')

    fetch(
      `${
        process.env.REACT_APP_API_URL
      }/api/feature_gates/promotion?release=${encodeURIComponent(
        release
      )}&feature_gate=${encodeURIComponent(featureGate)}`
    )
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
          'Could not retrieve promotion readiness: ' + error.message
        )
        setLoaded(true)
      })
  }, [release, featureGate])

  if (fetchError) {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    )
  }

  if (!data) {
    return (
      <Alert severity="warning">No promotion readiness data available.</Alert>
    )
  }

  const variants = data.variants || []

  // Collect all unique test names across all variants
  const testNameSet = new Set()
  for (const v of variants) {
    for (const tr of v.test_results || []) {
      testNameSet.add(tr.test_name)
    }
  }
  const allTestNames = Array.from(testNameSet)

  // Build a lookup: { variantKey: { testName: testResult } }
  const variantTestMap = {}
  for (const v of variants) {
    const key = variantKey(v)
    variantTestMap[key] = {}
    for (const tr of v.test_results || []) {
      variantTestMap[key][tr.test_name] = tr
    }
  }

  // Sort test names
  const sortedTestNames = [...allTestNames].sort((a, b) => {
    if (orderBy === 'test_name') {
      return order === 'asc' ? a.localeCompare(b) : b.localeCompare(a)
    }
    // Sort by a variant column's pass percent
    const varIdx = parseInt(orderBy, 10)
    if (!isNaN(varIdx) && variants[varIdx]) {
      const vKey = variantKey(variants[varIdx])
      const aResult = variantTestMap[vKey]?.[a]
      const bResult = variantTestMap[vKey]?.[b]
      const aVal = aResult ? aResult.pass_percent : -1
      const bVal = bResult ? bResult.pass_percent : -1
      return order === 'asc' ? aVal - bVal : bVal - aVal
    }
    return 0
  })

  const handleSort = (col) => {
    if (orderBy === col) {
      setOrder(order === 'asc' ? 'desc' : 'asc')
    } else {
      setOrderBy(col)
      setOrder('asc')
    }
  }

  const thresholds = {
    requiredTests: 5,
    requiredRuns: 14,
    requiredPassRate: 0.95,
  }

  const warnings = data.warnings || []
  const errors = data.errors || []

  return (
    <Box sx={{ mt: 2 }}>
      {data.sufficient ? (
        <Alert severity="success" sx={{ mb: 2 }}>
          Sufficient CI testing for &quot;{featureGate}&quot;.
        </Alert>
      ) : (
        <Alert severity="error" sx={{ mb: 2 }}>
          <strong>INSUFFICIENT</strong> CI testing for &quot;{featureGate}
          &quot;.
          <ul style={{ margin: '8px 0 0', paddingLeft: '20px' }}>
            <li>
              At least {thresholds.requiredTests} tests are expected for a
              feature
            </li>
            <li>Tests must be run on every TechPreview platform</li>
            <li>
              All tests must run at least {thresholds.requiredRuns} times on
              every platform
            </li>
            <li>
              All tests must pass at least{' '}
              {Math.round(thresholds.requiredPassRate * 100)}% of the time
            </li>
          </ul>
        </Alert>
      )}

      {warnings.length > 0 && (
        <Box sx={{ mb: 1 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              cursor: 'pointer',
            }}
            onClick={() => setShowWarnings(!showWarnings)}
          >
            <IconButton size="small">
              <ExpandMoreIcon
                sx={{
                  transform: showWarnings ? 'rotate(180deg)' : 'rotate(0deg)',
                  transition: 'transform 0.2s',
                }}
              />
            </IconButton>
            <Typography variant="body2" color="text.secondary">
              {warnings.length} warning{warnings.length !== 1 ? 's' : ''}
            </Typography>
          </Box>
          <Collapse in={showWarnings}>
            <Alert severity="warning" sx={{ mt: 0.5 }}>
              <ul style={{ margin: 0, paddingLeft: '20px' }}>
                {warnings.map((w, i) => (
                  <li key={i}>{w}</li>
                ))}
              </ul>
            </Alert>
          </Collapse>
        </Box>
      )}

      {errors.length > 0 && (
        <Box sx={{ mb: 1 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              cursor: 'pointer',
            }}
            onClick={() => setShowErrors(!showErrors)}
          >
            <IconButton size="small">
              <ExpandMoreIcon
                sx={{
                  transform: showErrors ? 'rotate(180deg)' : 'rotate(0deg)',
                  transition: 'transform 0.2s',
                }}
              />
            </IconButton>
            <Typography variant="body2" color="text.secondary">
              {errors.length} error{errors.length !== 1 ? 's' : ''}
            </Typography>
          </Box>
          <Collapse in={showErrors}>
            <Alert severity="error" sx={{ mt: 0.5 }}>
              <ul style={{ margin: 0, paddingLeft: '20px' }}>
                {errors.map((e, i) => (
                  <li key={i}>{e}</li>
                ))}
              </ul>
            </Alert>
          </Collapse>
        </Box>
      )}

      {variants.length === 0 ? (
        <Alert severity="info" sx={{ mt: 2 }}>
          No variant data available for this feature gate.
        </Alert>
      ) : (
        <TableContainer sx={{ mt: 2 }}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>
                  <TableSortLabel
                    active={orderBy === 'test_name'}
                    direction={orderBy === 'test_name' ? order : 'asc'}
                    onClick={() => handleSort('test_name')}
                  >
                    Test Name
                  </TableSortLabel>
                </TableCell>
                {variants.map((v, i) => (
                  <TableCell
                    key={i}
                    align="center"
                    sx={{ minWidth: 100, whiteSpace: 'nowrap' }}
                  >
                    <TableSortLabel
                      active={orderBy === String(i)}
                      direction={orderBy === String(i) ? order : 'asc'}
                      onClick={() => handleSort(String(i))}
                    >
                      {variantColumnHeader(v).map((part, pi) => (
                        <span key={pi}>
                          {pi > 0 && <br />}
                          {part}
                        </span>
                      ))}
                    </TableSortLabel>
                    {v.optional && (
                      <Chip
                        label="Optional"
                        size="small"
                        sx={{ ml: 0.5, fontSize: '0.7rem' }}
                      />
                    )}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {sortedTestNames.map((testName) => (
                <TableRow key={testName}>
                  <TableCell
                    sx={{
                      maxWidth: 500,
                      wordBreak: 'break-word',
                      fontSize: '0.85em',
                    }}
                  >
                    {testName}
                  </TableCell>
                  {variants.map((v, vi) => {
                    const vKey = variantKey(v)
                    const tr = variantTestMap[vKey]?.[testName]
                    return (
                      <PromotionCell
                        key={vi}
                        testResult={tr}
                        variant={v}
                        testName={testName}
                        release={release}
                        featureGate={featureGate}
                        onCellClick={onCellClick}
                      />
                    )
                  })}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  )
}

function PromotionCell({
  testResult,
  variant,
  testName,
  release,
  featureGate,
  onCellClick,
}) {
  if (!testResult) {
    return (
      <TableCell
        align="center"
        sx={{
          backgroundColor: FAIL_COLOR,
          cursor: onCellClick ? 'pointer' : 'default',
        }}
        onClick={() => onCellClick && onCellClick(variant, testName)}
      >
        <Typography variant="body2">
          <strong>FAIL</strong>
          <br />
          0% (0 / 0)
        </Typography>
      </TableCell>
    )
  }

  const passPercent = Math.round(testResult.pass_percent * 100)
  const isFailing = !testResult.sufficient
  const bgColor = isFailing ? FAIL_COLOR : PASS_COLOR

  return (
    <Tooltip
      title={`${testResult.successful_runs} passed, ${testResult.failed_runs} failed, ${testResult.flaked_runs} flaked out of ${testResult.total_runs} runs`}
    >
      <TableCell
        align="center"
        sx={{
          backgroundColor: bgColor,
          cursor: onCellClick ? 'pointer' : 'default',
        }}
        onClick={() => onCellClick && onCellClick(variant, testName)}
      >
        <Typography variant="body2">
          {isFailing && (
            <>
              <strong>FAIL</strong>
              <br />
            </>
          )}
          {passPercent}% ({testResult.successful_runs} / {testResult.total_runs}
          )
        </Typography>
      </TableCell>
    </Tooltip>
  )
}

function variantKey(v) {
  return [
    v.cloud,
    v.architecture,
    v.topology,
    v.network_stack || '',
    v.os || '',
  ].join('/')
}

FeatureGatePromotionTab.propTypes = {
  release: PropTypes.string.isRequired,
  featureGate: PropTypes.string.isRequired,
  onCellClick: PropTypes.func,
}

PromotionCell.propTypes = {
  testResult: PropTypes.object,
  variant: PropTypes.object.isRequired,
  testName: PropTypes.string.isRequired,
  release: PropTypes.string.isRequired,
  featureGate: PropTypes.string.isRequired,
  onCellClick: PropTypes.func,
}
