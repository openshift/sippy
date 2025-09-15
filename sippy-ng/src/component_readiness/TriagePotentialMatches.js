import { BooleanParam, useQueryParam } from 'use-query-params'
import {
  Button,
  Checkbox,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  FormGroup,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material'
import { Close } from '@mui/icons-material'
import { CompReadyVarsContext } from './CompReadyVars'
import { DataGrid, GridToolbar } from '@mui/x-data-grid'
import { generateTestDetailsReportLink } from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
import { relativeTime } from '../helpers'
import CompSeverityIcon from './CompSeverityIcon'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'

const useStyles = makeStyles((theme) => ({
  dialogPaper: {
    width: '90%',
    maxWidth: 'none',
  },
  loadingContainer: {
    textAlign: 'center',
    padding: '40px',
  },
  loadingText: {
    marginTop: '16px',
  },
  statusCell: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100%',
  },
  confidenceTooltip: {
    cursor: 'help',
    width: '100%',
    height: '100%',
    display: 'flex',
    alignItems: 'center',
  },
  dialogActions: {
    justifyContent: 'flex-start',
  },
  filterContainer: {
    marginBottom: theme.spacing(2),
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2),
  },
  filterGroup: {
    display: 'flex',
    flexDirection: 'row',
    gap: theme.spacing(2),
    alignItems: 'center',
  },
  filterTitle: {
    marginRight: theme.spacing(1),
    fontWeight: 'bold',
  },
}))

export default function TriagePotentialMatches({
  triage,
  setMessage,
  setLinkingComplete,
}) {
  const classes = useStyles()
  const [isModalOpen, setIsModalOpen] = React.useState(false)
  const [potentialMatches, setPotentialMatches] = React.useState([])
  const [isLoading, setIsLoading] = React.useState(false)
  const [selectedRegressions, setSelectedRegressions] = React.useState([])
  const [isLinking, setIsLinking] = React.useState(false)
  const [filterSimilarNames, setFilterSimilarNames] = React.useState(true)
  const [filterSameLastFailures, setFilterSameLastFailures] =
    React.useState(true)
  const { view, expandEnvironment } = useContext(CompReadyVarsContext)
  const [autoOpenMatches, setAutoOpenMatches] = useQueryParam(
    'openMatches',
    BooleanParam
  )

  React.useEffect(() => {
    if (autoOpenMatches === true) {
      findPotentialMatches()
    }
  }, [autoOpenMatches])

  const findPotentialMatches = () => {
    setIsLoading(true)
    setIsModalOpen(true)
    fetch(`${triage.links.potential_matches}?view=${view}`)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
      .then((matches) => {
        console.log('Potential matching regressions:', matches)
        setPotentialMatches(matches || [])
        setSelectedRegressions([])
        setFilterSimilarNames(true)
        setFilterSameLastFailures(true)
        setIsModalOpen(true)
      })
      .catch((error) => {
        console.error('Error finding potential matches:', error)
        setMessage('Error finding potential matches: ' + error.toString())
      })
      .finally(() => {
        setIsLoading(false)
      })
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setAutoOpenMatches(undefined)
  }

  const linkSelectedRegressions = () => {
    if (selectedRegressions.length === 0) {
      setMessage('No regressions selected')
      return
    }

    setIsLinking(true)

    // Update triage with selected regressions
    const updatedTriage = {
      ...triage,
      regressions: [
        ...triage.regressions,
        ...selectedRegressions.map((id) => ({ id: Number(id) })),
      ],
    }

    fetch(triage.links.self, {
      method: 'PUT',
      body: JSON.stringify(updatedTriage),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error('Failed to link regressions: ' + response.status)
        }
        setLinkingComplete(true)
        setAutoOpenMatches(undefined)
      })
      .catch((error) => {
        console.error('Error linking regressions:', error)
        setMessage('Error linking regressions: ' + error.message)
        setIsLinking(false)
      })
  }

  const filteredMatches = React.useMemo(() => {
    if (filterSimilarNames && filterSameLastFailures) {
      return potentialMatches
    }

    return potentialMatches.filter((match) => {
      const hasSimilarNames =
        match.similarly_named_tests && match.similarly_named_tests.length > 0
      const hasSameLastFailures =
        match.same_last_failures && match.same_last_failures.length > 0

      if (!filterSimilarNames && !filterSameLastFailures) {
        return false
      }
      if (filterSimilarNames && !filterSameLastFailures) {
        return hasSimilarNames
      }
      if (!filterSimilarNames && filterSameLastFailures) {
        return hasSameLastFailures
      }

      return false
    })
  }, [potentialMatches, filterSimilarNames, filterSameLastFailures])

  const columns = [
    {
      field: 'triage',
      headerName: 'Triage',
      flex: 4,
      sortable: false,
      filterable: false,
      disableColumnMenu: true,
      renderCell: (params) => (
        <input
          type="checkbox"
          checked={selectedRegressions.includes(
            params.row.regressed_test.regression.id
          )}
          onChange={(e) => {
            const regressionId = params.row.regressed_test.regression.id
            if (e.target.checked) {
              setSelectedRegressions([...selectedRegressions, regressionId])
            } else {
              setSelectedRegressions(
                selectedRegressions.filter((id) => id !== regressionId)
              )
            }
          }}
        />
      ),
    },
    {
      field: 'test_name',
      headerName: 'Test Name',
      flex: 60,
      valueGetter: (params) => {
        return params.row.regressed_test.test_name
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'release',
      headerName: 'Release',
      flex: 4,
      valueGetter: (params) => {
        return params.row.regressed_test.regression.release
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'variants',
      headerName: 'Variants',
      flex: 40,
      valueGetter: (params) => {
        const variants = params.row.regressed_test.regression.variants
        if (variants && Array.isArray(variants)) {
          return variants.join(', ')
        }
        return variants || ''
      },
      renderCell: (param) => <div className="test-name">{param.value}</div>,
    },
    {
      field: 'opened',
      headerName: 'Regressed Since',
      flex: 12,
      valueGetter: (params) => {
        const opened = params.row.regressed_test.regression.opened
        if (!opened) {
          return ''
        }
        const regressedSinceDate = new Date(opened)
        return relativeTime(regressedSinceDate, new Date())
      },
      renderCell: (param) => (
        <div className="regressed-since">{param.value}</div>
      ),
    },
    {
      field: 'status',
      headerName: 'Status',
      flex: 4,
      valueGetter: (params) => {
        const regressedTest = params.row.regressed_test
        const filterVals = `?view=${view}`
        //TODO: we need to get this off of the regression...
        const testDetailsUrl = generateTestDetailsReportLink(
          regressedTest,
          filterVals,
          expandEnvironment
        )

        return {
          status: regressedTest.status || 0,
          explanations: regressedTest.explanations || [],
          url: testDetailsUrl,
        }
      },
      renderCell: (params) => (
        <div className={classes.statusCell}>
          <a href={params.value.url} target="_blank" rel="noopener noreferrer">
            <CompSeverityIcon
              status={params.value.status}
              explanations={params.value.explanations}
            />
          </a>
        </div>
      ),
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
          <span>Confidence</span>
        </Tooltip>
      ),
      flex: 6,
      renderCell: (params) => {
        const row = params.row
        const similarlyNamedCount = row.similarly_named_tests
          ? row.similarly_named_tests.length
          : 0
        const sameLastFailureCount = row.same_last_failures
          ? row.same_last_failures.length
          : 0

        const tooltipContent = (
          <div>
            <div>Match Breakdown:</div>
            <div>• Similarly Named Tests: {similarlyNamedCount}</div>
            <div>• Same Last Failure: {sameLastFailureCount}</div>
          </div>
        )

        return (
          <Tooltip title={tooltipContent} arrow placement="top">
            <div className={classes.confidenceTooltip}>{params.value}</div>
          </Tooltip>
        )
      },
    },
  ]

  return (
    <Fragment>
      <Button
        onClick={findPotentialMatches}
        variant="contained"
        color="primary"
        sx={{ marginTop: '10px' }}
        disabled={isLoading}
      >
        {isLoading
          ? 'Finding Matches...'
          : 'Link Additional Matching Regressions'}
      </Button>

      <Dialog
        open={isModalOpen}
        onClose={handleCloseModal}
        maxWidth={false}
        classes={{
          paper: classes.dialogPaper,
        }}
      >
        <DialogTitle>
          Potential Matching Regressions
          <IconButton
            aria-label="close"
            onClick={handleCloseModal}
            sx={{ position: 'absolute', right: 8, top: 8 }}
          >
            <Close />
          </IconButton>
        </DialogTitle>
        <DialogContent>
          {isLoading ? (
            <div className={classes.loadingContainer}>
              <CircularProgress />
              <div className={classes.loadingText}>
                Loading potential matches...
              </div>
            </div>
          ) : (
            <>
              <div className={classes.filterContainer}>
                <Typography className={classes.filterTitle}>
                  Filter by:
                </Typography>
                <FormGroup className={classes.filterGroup}>
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={filterSimilarNames}
                        onChange={(e) =>
                          setFilterSimilarNames(e.target.checked)
                        }
                      />
                    }
                    label={`Similar Names (${
                      potentialMatches.filter(
                        (m) => m.similarly_named_tests?.length > 0
                      ).length
                    })`}
                  />
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={filterSameLastFailures}
                        onChange={(e) =>
                          setFilterSameLastFailures(e.target.checked)
                        }
                      />
                    }
                    label={`Same Last Failures (${
                      potentialMatches.filter(
                        (m) => m.same_last_failures?.length > 0
                      ).length
                    })`}
                  />
                </FormGroup>
              </div>
              <DataGrid
                rows={filteredMatches}
                columns={columns}
                components={{ Toolbar: GridToolbar }}
                getRowId={(row) => row.regressed_test.regression.id}
                autoHeight
                rowHeight={80}
                pageSize={10}
                rowsPerPageOptions={[10, 25, 50]}
                disableSelectionOnClick
                componentsProps={{
                  toolbar: {
                    columns: columns,
                  },
                }}
              />
            </>
          )}
        </DialogContent>
        <DialogActions className={classes.dialogActions}>
          <Button
            onClick={linkSelectedRegressions}
            disabled={selectedRegressions.length === 0 || isLinking}
            variant="contained"
            color="primary"
          >
            {isLinking
              ? 'Linking...'
              : `Add ${
                  selectedRegressions.length > 0
                    ? `(${selectedRegressions.length})`
                    : ''
                } to Triage`}
          </Button>
          <Button onClick={handleCloseModal} color="secondary">
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Fragment>
  )
}

TriagePotentialMatches.propTypes = {
  triage: PropTypes.object.isRequired,
  setMessage: PropTypes.func.isRequired,
  setLinkingComplete: PropTypes.func.isRequired,
}
