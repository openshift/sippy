import { Button, Tooltip, Typography } from '@mui/material'
import {
  CheckCircle,
  Error as ErrorIcon,
  Info as InfoIcon,
} from '@mui/icons-material'
import { DataGrid } from '@mui/x-data-grid'
import { formatDateToSeconds, relativeTime } from '../helpers'
import {
  getRegressionAPIUrl,
  getTriagesAPIUrl,
  jiraUrlPrefix,
} from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
import { useTheme } from '@mui/material/styles'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect, useState } from 'react'

const useStyles = makeStyles((theme) => ({
  ellipsisText: {
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  customTooltip: {
    maxWidth: 600,
    fontSize: '0.875rem',
    whiteSpace: 'pre-line',
  },
  centeredHeading: {
    textAlign: 'center',
  },
  successIcon: {
    color: theme.palette.success.light,
  },
  errorIcon: {
    color: theme.palette.error.light,
  },
}))

export default function RegressionPotentialMatchesTab({
  regressionId,
  setAlertText,
  setAlertSeverity,
  completeTriageSubmission,
  onMatchesFound,
}) {
  const theme = useTheme()
  const classes = useStyles()
  const [matches, setMatches] = useState([])
  const [loadingMatches, setLoadingMatches] = useState(false)
  const [matchesError, setMatchesError] = useState('')

  const fetchMatchingTriages = () => {
    if (!regressionId) {
      setMatchesError('No regression ID provided')
      return
    }

    setLoadingMatches(true)
    setMatchesError('')

    const matchesApiCall = getRegressionAPIUrl(regressionId) + '/matches'
    fetch(matchesApiCall)
      .then((response) => response.json())
      .then((data) => {
        if (data && data.code && (data.code < 200 || data.code >= 300)) {
          const errorMessage = data.message
            ? `${data.message}`
            : 'No error message'
          throw new Error(
            `API call failed: ${matchesApiCall}\n Return code = ${data.code} (${errorMessage})`
          )
        }
        if (!data) {
          setMatches([])
          onMatchesFound(false)
          return
        }
        setMatches(data)
        onMatchesFound(data.length > 0)
      })
      .catch((error) => {
        setMatchesError(error.message || 'Failed to fetch matching triages')
      })
      .finally(() => {
        setLoadingMatches(false)
      })
  }

  useEffect(() => {
    if (regressionId) {
      fetchMatchingTriages()
    }
  }, [regressionId])

  const renderCountWithTooltip = (items) => {
    const count = items.length

    if (count === 0) {
      return <Typography variant="body2">0</Typography>
    }

    const testNames = items.map((item) => item.test_name)
    const displayNames = testNames.slice(0, 10)
    const tooltipText =
      displayNames.join('\n') + (testNames.length > 10 ? '\n...' : '')

    return (
      <Tooltip
        title={tooltipText}
        placement="top"
        classes={{ tooltip: classes.customTooltip }}
      >
        <Typography variant="body2">{count}</Typography>
      </Tooltip>
    )
  }

  const handleAddToTriage = (triage) => {
    const updatedTriage = {
      ...triage,
      regressions: [...triage.regressions, { id: Number(regressionId) }],
    }

    fetch(getTriagesAPIUrl(triage.id), {
      method: 'PUT',
      body: JSON.stringify(updatedTriage),
    })
      .then((response) => {
        if (!response.ok) {
          response.json().then((data) => {
            let errorMessage = 'invalid response returned from server'
            if (data?.code) {
              errorMessage =
                'error adding test to triage entry: ' +
                data.code +
                ': ' +
                data.message
            }
            console.error(errorMessage)
            setAlertText(errorMessage)
            setAlertSeverity('error')
          })
          return
        }

        setAlertText('Successfully added regression to triage: ' + triage.url)
        setAlertSeverity('success')
        // This will reload the page
        completeTriageSubmission()
      })
      .catch((error) => {
        setAlertText('Error adding regression to triage: ' + error.message)
        setAlertSeverity('error')
      })
  }

  const columns = [
    {
      field: 'resolution_date',
      valueGetter: (params) => {
        return params.row.triage.resolved?.Valid
          ? params.row.triage.resolved.Time
          : ''
      },
      headerName: 'Resolved',
      flex: 4,
      align: 'center',
      renderCell: (param) =>
        param.value ? (
          <Tooltip
            title={`${relativeTime(
              new Date(param.value),
              new Date()
            )} (${formatDateToSeconds(param.value)})`}
          >
            <CheckCircle className={classes.successIcon} />
          </Tooltip>
        ) : (
          <Tooltip title="Not resolved">
            <ErrorIcon className={classes.errorIcon} />
          </Tooltip>
        ),
    },
    {
      field: 'description',
      headerName: 'Description',
      flex: 25,
      valueGetter: (params) => {
        return params.row.triage.description
      },
      renderCell: (param) => (
        <Tooltip title={param.value || 'No description'}>
          <Typography className={classes.ellipsisText}>
            {param.value || 'No description'}
          </Typography>
        </Tooltip>
      ),
    },
    {
      field: 'type',
      headerName: 'Type',
      flex: 8,
      valueGetter: (params) => {
        return params.row.triage.type
      },
      renderCell: (param) => (
        <Typography variant="body2">{param.value}</Typography>
      ),
    },
    {
      field: 'url',
      valueGetter: (params) => {
        const url = params.row.triage.url
        const val = {
          url,
          text: url,
        }
        if (url && url.startsWith(jiraUrlPrefix)) {
          val.text = url.slice(jiraUrlPrefix.length)
        }
        return val
      },
      headerName: 'Jira',
      flex: 5,
      renderCell: (param) => (
        <a target="_blank" href={param.value.url} rel="noreferrer">
          <div className="test-name">{param.value.text}</div>
        </a>
      ),
    },
    {
      field: 'similar_tests',
      headerName: 'Similar Tests',
      flex: 6,
      align: 'center',
      valueGetter: (params) => params.row.similarly_named_tests || [],
      renderCell: (param) => {
        const tests = param.value.map((test) => {
          return test.regression
        })
        return renderCountWithTooltip(tests)
      },
    },
    {
      field: 'same_failures',
      headerName: 'Same Last Failure',
      flex: 6,
      align: 'center',
      valueGetter: (params) => params.row.same_last_failures || [],
      renderCell: (param) => {
        const failures = param.value
        return renderCountWithTooltip(failures)
      },
    },
    {
      field: 'confidence_level',
      headerName: (
        <Tooltip
          title={
            'Confidence Level (0-10) - Higher values indicate higher likelihood of matching based on: Similar test names (edit distance scoring), Same last failure times (fails in the same job runs)'
          }
          arrow
          placement="top"
        >
          <span>Confidence Level</span>
        </Tooltip>
      ),
      flex: 6,
      align: 'center',
      renderCell: (param) => (
        <Typography variant="body2">{param.value}/10</Typography>
      ),
    },
    {
      field: 'details',
      headerName: 'Details',
      flex: 4,
      align: 'center',
      sortable: false,
      valueGetter: (value) => {
        return value.row.triage.id
      },
      renderCell: (param) => (
        <a
          href={'/sippy-ng/component_readiness/triages/' + param.value}
          target="_blank"
          rel="noopener noreferrer"
        >
          <InfoIcon />
        </a>
      ),
    },
    {
      field: 'add_to_triage',
      headerName: '',
      flex: 10,
      align: 'center',
      sortable: false,
      renderCell: (param) => (
        <Button
          variant="contained"
          color="primary"
          onClick={() => handleAddToTriage(param.row.triage)}
          size="small"
        >
          Add to Triage
        </Button>
      ),
    },
  ]

  return (
    <Fragment>
      <h3 className={classes.centeredHeading}>Potential Matching Triages</h3>
      {loadingMatches && <Typography>Loading matching triages...</Typography>}
      {matchesError && (
        <Typography color="error">Error: {matchesError}</Typography>
      )}
      {!loadingMatches && !matchesError && (
        <DataGrid
          sortModel={[{ field: 'confidence_level', sort: 'desc' }]}
          rows={matches}
          columns={columns}
          getRowId={(row) => row.triage.id}
          pageSize={Math.max(3, matches.length)}
          rowHeight={100}
          autoHeight={true}
          hideFooterPagination={matches.length <= 3}
          hideFooter={matches.length <= 3}
          disableSelectionOnClick
          disableColumnMenu
          disableColumnFilter
          disableColumnSelector
          disableColumnSorting
        />
      )}
    </Fragment>
  )
}

RegressionPotentialMatchesTab.propTypes = {
  regressionId: PropTypes.number.isRequired,
  setAlertText: PropTypes.func.isRequired,
  setAlertSeverity: PropTypes.func.isRequired,
  completeTriageSubmission: PropTypes.func.isRequired,
  onMatchesFound: PropTypes.func.isRequired,
}
