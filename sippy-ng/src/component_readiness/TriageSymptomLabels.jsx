import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  LinearProgress,
  Tooltip,
  Typography,
} from '@mui/material'
import { filterFor, pathForJobRunsWithFilter } from '../helpers'
import { FilterList } from '@mui/icons-material'
import { Link } from 'react-router-dom'
import { symptomColor } from './CompReadyUtils'
import PropTypes from 'prop-types'
import React from 'react'
import ReactMarkdown from 'react-markdown'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'

export function aggregateLabelSummaries(jobRuns, labels, totalRegressions) {
  const labelLookup = Object.fromEntries(
    labels.map((label) => [label.id, label])
  )
  const summaries = new Map()

  for (const run of jobRuns) {
    for (const labelID of new Set(run.job_labels || [])) {
      if (!summaries.has(labelID)) {
        const label = labelLookup[labelID]
        summaries.set(labelID, {
          label: {
            id: labelID,
            label_title: label?.label_title || labelID,
            explanation: label?.explanation || '',
          },
          regression_ids: new Set(),
          job_run_count: 0,
        })
      }

      const summary = summaries.get(labelID)
      summary.job_run_count++
      if (run.regression_id !== undefined) {
        summary.regression_ids.add(Number(run.regression_id))
      }
    }
  }

  return Array.from(summaries.values())
    .map((summary) => {
      const regressionIDs = Array.from(summary.regression_ids)
      const showRegressions = totalRegressions !== undefined
      const numerator = showRegressions
        ? regressionIDs.length
        : summary.job_run_count
      const denominator = showRegressions ? totalRegressions : jobRuns.length
      return {
        label: summary.label,
        regression_ids: regressionIDs,
        regression_count: showRegressions ? regressionIDs.length : undefined,
        job_run_count: summary.job_run_count,
        percentage: denominator > 0 ? (numerator / denominator) * 100 : 0,
      }
    })
    .sort(
      (a, b) =>
        b.job_run_count - a.job_run_count ||
        a.label.label_title.localeCompare(b.label.label_title)
    )
}

export function filterRegressionsByLabel(
  regressions,
  labelFilter,
  labelSummaries
) {
  if (!labelFilter || !labelSummaries) {
    return regressions
  }

  const summary = labelSummaries.find(
    (candidate) => candidate.label.id === labelFilter
  )
  if (!summary) {
    return regressions
  }

  const matchingRegressionIDs = new Set(
    (summary.regression_ids || []).map(Number)
  )
  return regressions.filter((regression) =>
    matchingRegressionIDs.has(regression.id)
  )
}

export default function TriageSymptomLabels({
  labelFilter,
  labelSummaries,
  release,
  setLabelFilter,
}) {
  const [selectedLabel, setSelectedLabel] = React.useState(null)
  const showRegressions = labelSummaries?.some(
    (summary) => summary.regression_count !== undefined
  )
  const selectedLabelJobRunsPath =
    selectedLabel && release
      ? pathForJobRunsWithFilter(release, {
          items: [filterFor('labels', 'has entry', selectedLabel.id)],
        })
      : null

  return (
    <>
      <Tooltip
        title="Labels identify known conditions found in failed job runs."
        followCursor
      >
        <h2 style={{ cursor: 'help' }}>
          Failure Labels
          {labelSummaries?.length > 0 && ` (${labelSummaries.length})`}
        </h2>
      </Tooltip>
      {!labelSummaries || labelSummaries.length === 0 ? (
        <Typography color="text.secondary" sx={{ mb: 2 }}>
          No labels applied
        </Typography>
      ) : (
        <Table size="small" sx={{ mb: 3 }}>
          <TableHead>
            <TableRow>
              <TableCell>Label</TableCell>
              {showRegressions && <TableCell>Regressions</TableCell>}
              <TableCell sx={{ minWidth: 120 }}>
                <Tooltip
                  title={
                    showRegressions
                      ? 'Percentage of regressions in this triage with a failed job run carrying this label'
                      : 'Percentage of failed job runs carrying this label'
                  }
                >
                  <span>Percentage</span>
                </Tooltip>
              </TableCell>
              <TableCell>Failed Runs</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {labelSummaries.map((summary) => {
              const jobRunsPath = release
                ? pathForJobRunsWithFilter(release, {
                    items: [filterFor('labels', 'has entry', summary.label.id)],
                  })
                : null
              const labelChip = (
                <Chip
                  label={summary.label.label_title}
                  size="small"
                  onClick={() => setSelectedLabel(summary.label)}
                  sx={{
                    backgroundColor: symptomColor(summary.label.id),
                    color: '#fff',
                    fontSize: '0.75rem',
                  }}
                />
              )

              return (
                <TableRow key={summary.label.id}>
                  <TableCell>
                    <Box display="flex" alignItems="center" gap={0.5}>
                      {labelChip}
                      {jobRunsPath && (
                        <Tooltip title="View job runs with this label">
                          <IconButton
                            component={Link}
                            to={jobRunsPath}
                            target="_blank"
                            rel="noopener noreferrer"
                            size="small"
                            aria-label="View job runs with this label"
                          >
                            <FilterList fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Box>
                  </TableCell>
                  {showRegressions && (
                    <TableCell>
                      <Box display="flex" alignItems="center" gap={0.5}>
                        {summary.regression_count}
                        {setLabelFilter && (
                          <Tooltip title="Filter regressions to this label">
                            <IconButton
                              size="small"
                              aria-label={`Filter regressions to ${summary.label.label_title}`}
                              aria-pressed={labelFilter === summary.label.id}
                              onClick={() =>
                                setLabelFilter(
                                  labelFilter === summary.label.id
                                    ? null
                                    : summary.label.id
                                )
                              }
                              color={
                                labelFilter === summary.label.id
                                  ? 'primary'
                                  : 'default'
                              }
                            >
                              <FilterList fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        )}
                      </Box>
                    </TableCell>
                  )}
                  <TableCell>
                    <Box display="flex" alignItems="center" gap={1}>
                      <LinearProgress
                        variant="determinate"
                        value={summary.percentage}
                        sx={{ flexGrow: 1, height: 6, borderRadius: 3 }}
                      />
                      <Typography variant="caption" sx={{ minWidth: 40 }}>
                        {summary.percentage.toFixed(1)}%
                      </Typography>
                    </Box>
                  </TableCell>
                  <TableCell>{summary.job_run_count}</TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
      <Dialog
        open={selectedLabel !== null}
        onClose={() => setSelectedLabel(null)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          {selectedLabel?.label_title || 'Label details'}
        </DialogTitle>
        <DialogContent>
          {selectedLabel?.explanation ? (
            <ReactMarkdown>{selectedLabel.explanation}</ReactMarkdown>
          ) : (
            <Typography color="text.secondary">
              No description available.
            </Typography>
          )}
          {selectedLabelJobRunsPath ? (
            <Button
              component={Link}
              to={selectedLabelJobRunsPath}
              target="_blank"
              rel="noopener noreferrer"
              sx={{ mt: 1 }}
            >
              View job runs with this label
            </Button>
          ) : (
            <Typography color="text.secondary" sx={{ mt: 1 }}>
              Job run link unavailable.
            </Typography>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSelectedLabel(null)}>Close</Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

TriageSymptomLabels.propTypes = {
  labelFilter: PropTypes.string,
  labelSummaries: PropTypes.array,
  release: PropTypes.string,
  setLabelFilter: PropTypes.func,
}
