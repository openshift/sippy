import {
  Box,
  Chip,
  IconButton,
  LinearProgress,
  Tooltip,
  Typography,
} from '@mui/material'
import { FilterList } from '@mui/icons-material'
import { symptomColor } from './CompReadyUtils'
import PropTypes from 'prop-types'
import React from 'react'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'

export default function TriageSymptoms({
  symptomSummaries,
  symptomFilter,
  setSymptomFilter,
}) {
  const showRegressions =
    symptomSummaries?.length > 0 &&
    symptomSummaries[0].regression_count !== undefined

  return (
    <>
      <Tooltip
        title="Symptoms are synced periodically by the regression-cache loader and may not appear immediately after a regression is added."
        followCursor
      >
        <h2 style={{ cursor: 'help' }}>
          Symptoms
          {symptomSummaries?.length > 0 && ` (${symptomSummaries.length})`}
        </h2>
      </Tooltip>
      {!symptomSummaries || symptomSummaries.length === 0 ? (
        <Typography color="text.secondary" sx={{ mb: 2 }}>
          No symptoms detected
        </Typography>
      ) : (
        <Table size="small" sx={{ mb: 3 }}>
          <TableHead>
            <TableRow>
              <TableCell>
                <Tooltip title="Auto-detected pattern found in job run artifacts">
                  <span>Symptom</span>
                </Tooltip>
              </TableCell>
              {showRegressions && (
                <TableCell>
                  <Tooltip title="Number of regressions in this triage where the symptom was detected">
                    <span>Regressions</span>
                  </Tooltip>
                </TableCell>
              )}
              <TableCell sx={{ minWidth: 120 }}>
                <Tooltip
                  title={
                    showRegressions
                      ? 'Percentage of regressions in this triage exhibiting the symptom'
                      : 'Percentage of failed job runs where the symptom was detected'
                  }
                >
                  <span>Percentage</span>
                </Tooltip>
              </TableCell>
              <TableCell>
                <Tooltip
                  title={
                    showRegressions
                      ? 'Total number of failed job runs across all regressions where the symptom was detected'
                      : 'Number of failed job runs where the symptom was detected'
                  }
                >
                  <span>Failed Runs</span>
                </Tooltip>
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {symptomSummaries.map((ss) => (
              <TableRow key={ss.symptom.id}>
                <TableCell>
                  <Chip
                    label={ss.symptom.summary}
                    size="small"
                    sx={{
                      backgroundColor: symptomColor(ss.symptom.id),
                      color: '#fff',
                      fontSize: '0.75rem',
                    }}
                  />
                </TableCell>
                {showRegressions && (
                  <TableCell>
                    <Box display="flex" alignItems="center" gap={1}>
                      {ss.regression_count}
                      <Tooltip title="Filter regressions to this symptom">
                        <IconButton
                          size="small"
                          aria-label={`Filter regressions to ${
                            ss.symptom.summary || ss.symptom.id
                          }`}
                          aria-pressed={symptomFilter === ss.symptom.id}
                          onClick={() =>
                            setSymptomFilter(
                              symptomFilter === ss.symptom.id
                                ? null
                                : ss.symptom.id
                            )
                          }
                          color={
                            symptomFilter === ss.symptom.id
                              ? 'primary'
                              : 'default'
                          }
                        >
                          <FilterList fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </TableCell>
                )}
                <TableCell>
                  <Box display="flex" alignItems="center" gap={1}>
                    <LinearProgress
                      variant="determinate"
                      value={ss.percentage}
                      sx={{ flexGrow: 1, height: 6, borderRadius: 3 }}
                    />
                    <Typography variant="caption" sx={{ minWidth: 40 }}>
                      {ss.percentage.toFixed(1)}%
                    </Typography>
                  </Box>
                </TableCell>
                <TableCell>{ss.job_run_count}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </>
  )
}

TriageSymptoms.propTypes = {
  symptomSummaries: PropTypes.array,
  symptomFilter: PropTypes.string,
  setSymptomFilter: PropTypes.func,
}
