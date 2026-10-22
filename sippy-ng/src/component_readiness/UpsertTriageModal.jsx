import {
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Snackbar,
  Tooltip,
  Typography,
} from '@mui/material'
import { getTriagesAPIUrl } from './CompReadyUtils'
import { makeStyles } from '../makeStyles'
import AddRegressionPanel from './AddRegressionPanel'
import Alert from '@mui/material/Alert'
import Dialog from '@mui/material/Dialog'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import UpdateTriagePanel from './UpdateTriagePanel'

const useCommonTriageStyles = makeStyles((theme) => ({
  tabsContainer: {
    borderBottom: `1px solid ${theme.palette.divider}`,
    '& .MuiTab-root': {
      border: `1px solid ${theme.palette.divider}`,
      borderBottom: 'none',
      margin: '0 2px',
      '&:hover': {
        backgroundColor: theme.palette.action.hover,
      },
      '&.Mui-selected': {
        backgroundColor: theme.palette.background.paper,
        borderColor: theme.palette.primary.main,
      },
    },
  },
  dialogContent: {
    overflow: 'auto',
    flex: 1,
    paddingTop: '8px',
    paddingBottom: '8px',
  },
  centeredHeading: {
    textAlign: 'center',
  },
  loadingContainer: {
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(2),
    justifyContent: 'center',
    minHeight: '400px',
  },
}))

export default function UpsertTriageModal({
  regressionIds,
  triage,
  setComplete,
  buttonText,
  submissionDelay = 0,
  fields = [],
}) {
  const commonClasses = useCommonTriageStyles()
  const regressionAddMode =
    regressionIds !== undefined && regressionIds.length > 0
  const triageDetailsUpdateMode = triage !== undefined

  const [triages, setTriages] = React.useState([])
  const [triagesLoading, setTriagesLoading] = React.useState(false)
  const [triageModalOpen, setTriageModalOpen] = React.useState(false)
  const triagesRequestGeneration = React.useRef(0)
  const handleTriageModalOpen = () => {
    // Updating uses the triage supplied by the parent, not the triage list.
    if (triageDetailsUpdateMode) {
      setTriageModalOpen(true)
      return
    }

    const requestGeneration = ++triagesRequestGeneration.current
    const triagesAPIUrl = getTriagesAPIUrl(null, null, fields)
    setTriagesLoading(true)
    setTriageModalOpen(true)
    // Only get existing entries when actually adding regressions
    fetch(triagesAPIUrl)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error(
            `API call failed ${triagesAPIUrl} Returned + ${response.status}`
          )
        }
        return response.json()
      })
      .then((triages) => {
        if (requestGeneration !== triagesRequestGeneration.current) {
          return
        }
        if (regressionAddMode) {
          // Test Details omits regressions from its compact response, so no triages are filtered out there.
          triages = triages.filter(
            (triage) =>
              !(triage.regressions || []).some((regression) =>
                regressionIds.includes(regression.id)
              )
          )
        }
        setTriages(triages)
        setTriagesLoading(false)
      })
      .catch((error) => {
        if (requestGeneration !== triagesRequestGeneration.current) {
          return
        }
        setTriagesLoading(false)
        setTriageModalOpen(false)
        setAlertText('Error retrieving existing triage records')
        setAlertSeverity('error')
        console.error(error)
      })
  }
  const handleTriageModalClosed = () => {
    ++triagesRequestGeneration.current
    setTriageModalOpen(false)
  }

  const handleTriageFormCompletion = () => {
    setTriageEntryData({
      url: '',
      type: 'type',
      description: '',
    })
    setTriageModalOpen(false)
    completeTriageSubmission()
  }

  const completeTriageSubmission = () => {
    setTriageModalOpen(false)
    // allow for an optional delay  to view the success message before the page gets reloaded
    const timer = setTimeout(() => {
      setComplete(true)
    }, submissionDelay)
    return () => clearTimeout(timer)
  }

  let initialTriage = {
    url: '',
    type: 'type',
    description: '',
  }
  if (triageDetailsUpdateMode) {
    initialTriage = triage
  }
  const [triageEntryData, setTriageEntryData] = React.useState(initialTriage)

  // when the regressionIds prop is changed in the parent, we must update the triageEntryData to reflect that
  useEffect(() => {
    if (regressionAddMode) {
      setTriageEntryData({
        ...triageEntryData,
        ids: regressionIds,
      })
    }
  }, [regressionIds])

  const [alertText, setAlertText] = React.useState('')
  const [alertSeverity, setAlertSeverity] = React.useState('success')
  const handleAlertClose = (event, reason) => {
    if (reason === 'clickaway') {
      return
    }
    setAlertText('')
    setAlertSeverity('success')
  }

  const openModalDisabled =
    !triageDetailsUpdateMode && regressionIds.length === 0
  const openModalTooltip = openModalDisabled
    ? 'Please select at least one regression to triage'
    : ''

  return (
    <Fragment>
      <Snackbar
        open={alertText.length > 0}
        autoHideDuration={10000}
        onClose={handleAlertClose}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        <Alert onClose={handleAlertClose} severity={alertSeverity}>
          {alertText}
        </Alert>
      </Snackbar>
      {!triageModalOpen && (
        <Tooltip title={openModalTooltip}>
          <span>
            <Button
              sx={{ margin: '10px 0' }}
              variant="contained"
              onClick={handleTriageModalOpen}
              disabled={openModalDisabled}
            >
              {buttonText}
            </Button>
          </span>
        </Tooltip>
      )}
      {triageModalOpen && (
        <Dialog
          aria-label="Triage"
          fullWidth
          maxWidth="md"
          open={triageModalOpen}
          onClose={handleTriageModalClosed}
          PaperProps={{
            style: {
              width: '1500px',
              maxWidth: 'none',
              minHeight: '500px',
              maxHeight: '800px',
            },
          }}
        >
          {triagesLoading && (
            <Fragment>
              <DialogTitle>Triage</DialogTitle>
              <DialogContent className={commonClasses.dialogContent}>
                <Box className={commonClasses.loadingContainer}>
                  <CircularProgress aria-label="Loading triage information..." />
                  <Typography>Loading triage information...</Typography>
                </Box>
              </DialogContent>
            </Fragment>
          )}
          {!triagesLoading && triageDetailsUpdateMode && (
            <UpdateTriagePanel
              triage={triage}
              setAlertText={setAlertText}
              setAlertSeverity={setAlertSeverity}
              triageEntryData={triageEntryData}
              setTriageEntryData={setTriageEntryData}
              handleTriageFormCompletion={handleTriageFormCompletion}
              commonClasses={commonClasses}
            />
          )}
          {!triagesLoading && regressionAddMode && (
            <AddRegressionPanel
              triages={triages}
              regressionIds={regressionIds}
              triageEntryData={triageEntryData}
              setTriageEntryData={setTriageEntryData}
              setAlertText={setAlertText}
              setAlertSeverity={setAlertSeverity}
              handleNewTriageFormCompletion={handleTriageFormCompletion}
              completeTriageSubmission={completeTriageSubmission}
              commonClasses={commonClasses}
            />
          )}
          <DialogActions sx={{ justifyContent: 'flex-start' }}>
            <Button
              variant="contained"
              color="secondary"
              onClick={handleTriageModalClosed}
            >
              CLOSE
            </Button>
          </DialogActions>
        </Dialog>
      )}
    </Fragment>
  )
}

UpsertTriageModal.propTypes = {
  regressionIds: PropTypes.array,
  triage: PropTypes.object,
  setComplete: PropTypes.func.isRequired,
  buttonText: PropTypes.string.isRequired,
  submissionDelay: PropTypes.number,
  fields: PropTypes.arrayOf(PropTypes.string),
}
