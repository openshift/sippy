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
const PASS_TEXT = '#155724'
const FAIL_COLOR = '#f8d7da'
const FAIL_TEXT = '#721c24'

function topologyDisplayName(topology) {
  if (topology === 'external') return 'hypershift'
  return topology
}

function variantColumnHeader(variant) {
  const v = variant.variants || {}
  const parts = [
    topologyDisplayName(v['Topology'] || ''),
    v['Platform'] || '',
    v['Architecture'] || '',
  ]
  if (v['NetworkStack']) parts.push(v['NetworkStack'])
  if (v['OS']) parts.push(v['OS'])
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

  const variants = data.results_by_variant || []

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

  const actualTestCount = allTestNames.length
  const requiredVariants = variants.filter((v) => !v.optional)
  const variantsWithCoverage = requiredVariants.filter(
    (v) => (v.test_results || []).length > 0
  ).length
  let minRuns = Infinity
  let minPassRate = Infinity
  for (const v of requiredVariants) {
    for (const tr of v.test_results || []) {
      if (tr.total_runs < minRuns) minRuns = tr.total_runs
      if (tr.pass_percent < minPassRate) minPassRate = tr.pass_percent
    }
  }
  if (!isFinite(minRuns)) minRuns = 0
  if (!isFinite(minPassRate)) minPassRate = 0

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
            {actualTestCount < thresholds.requiredTests && (
              <li>
                At least {thresholds.requiredTests} tests are expected for a
                feature (found {actualTestCount})
              </li>
            )}
            {variantsWithCoverage < requiredVariants.length && (
              <li>
                Tests must be run on every TechPreview platform (
                {variantsWithCoverage} of {requiredVariants.length} have
                coverage)
              </li>
            )}
            {minRuns < thresholds.requiredRuns && (
              <li>
                All tests must run at least {thresholds.requiredRuns} times on
                every platform (minimum {minRuns})
              </li>
            )}
            {minPassRate < thresholds.requiredPassRate && (
              <li>
                All tests must pass at least{' '}
                {Math.round(thresholds.requiredPassRate * 100)}% of the time
                (minimum {Math.round(minPassRate * 100)}%)
              </li>
            )}
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
                  <TableCell key={i} align="center" sx={{ minWidth: 80 }}>
                    <TableSortLabel
                      active={orderBy === String(i)}
                      direction={orderBy === String(i) ? order : 'asc'}
                      onClick={() => handleSort(String(i))}
                    >
                      <span style={{ whiteSpace: 'normal' }}>
                        {variantColumnHeader(v).map((part, pi) => (
                          <span key={pi}>
                            {pi > 0 && <br />}
                            {part}
                          </span>
                        ))}
                      </span>
                    </TableSortLabel>
                    {v.optional && (
                      <Chip
                        label="Optional"
                        size="small"
                        color="info"
                        sx={{ mt: 0.5, fontSize: '0.7rem', display: 'block' }}
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
                      py: 1.5,
                      pr: 2,
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
                        requiredRuns={thresholds.requiredRuns}
                        requiredPassRate={thresholds.requiredPassRate}
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

function cellTooltip(testResult, requiredRuns, requiredPassRate) {
  if (!testResult) {
    return 'No test data found for this variant'
  }
  const reasons = []
  if (testResult.total_runs < requiredRuns) {
    reasons.push(
      `Only ${testResult.total_runs} runs (need at least ${requiredRuns})`
    )
  }
  if (testResult.total_runs > 0 && testResult.pass_percent < requiredPassRate) {
    reasons.push(
      `Pass rate ${Math.round(
        testResult.pass_percent * 100
      )}% (need at least ${Math.round(requiredPassRate * 100)}%)`
    )
  }
  const stats = `${testResult.successful_runs} passed, ${testResult.failed_runs} failed, ${testResult.flaked_runs} flaked out of ${testResult.total_runs} runs`
  if (reasons.length === 0) {
    return stats
  }
  return `${reasons.join('. ')}. ${stats}`
}

function PromotionCell({
  testResult,
  variant,
  testName,
  release,
  featureGate,
  onCellClick,
  requiredRuns,
  requiredPassRate,
}) {
  if (!testResult) {
    return (
      <Tooltip title={cellTooltip(null, requiredRuns, requiredPassRate)}>
        <TableCell
          align="center"
          sx={{
            backgroundColor: FAIL_COLOR,
            color: FAIL_TEXT,
            cursor: onCellClick ? 'pointer' : 'default',
          }}
          onClick={() => onCellClick && onCellClick(variant, testName)}
        >
          <Typography variant="body2" color="inherit">
            <strong>FAIL</strong>
            <br />
            0% (0 / 0)
          </Typography>
        </TableCell>
      </Tooltip>
    )
  }

  const passPercent = Math.round(testResult.pass_percent * 100)
  const isFailing = !testResult.sufficient
  const lowRuns = testResult.total_runs < requiredRuns
  const lowPassRate =
    testResult.total_runs > 0 && testResult.pass_percent < requiredPassRate
  const isWarning = isFailing && lowRuns && !lowPassRate

  let bgColor = PASS_COLOR
  let textColor = PASS_TEXT
  let label = 'PASS'
  if (isFailing) {
    bgColor = FAIL_COLOR
    textColor = FAIL_TEXT
    label = isWarning ? 'LOW RUNS' : 'FAIL'
  }

  return (
    <Tooltip title={cellTooltip(testResult, requiredRuns, requiredPassRate)}>
      <TableCell
        align="center"
        sx={{
          backgroundColor: bgColor,
          color: textColor,
          cursor: onCellClick ? 'pointer' : 'default',
        }}
        onClick={() => onCellClick && onCellClick(variant, testName)}
      >
        <Typography variant="body2" color="inherit">
          <strong>{label}</strong>
          <br />
          {passPercent}% ({testResult.successful_runs} / {testResult.total_runs}
          )
        </Typography>
      </TableCell>
    </Tooltip>
  )
}

function variantKey(v) {
  const vars = v.variants || {}
  return Object.keys(vars)
    .sort()
    .map((k) => `${k}:${vars[k]}`)
    .join('/')
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
  requiredRuns: PropTypes.number.isRequired,
  requiredPassRate: PropTypes.number.isRequired,
}
