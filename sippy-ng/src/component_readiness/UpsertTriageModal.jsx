import {
  Button,
  CircularProgress,
  DialogActions,
  DialogTitle,
  Snackbar,
  Tooltip,
  Typography,
} from '@mui/material'
import { getTriagesAPIUrl } from './CompReadyUtils'
import { LoadingContainer, TriageDialogContent } from './TriageStyledComponents'
import AddRegressionPanel from './AddRegressionPanel'
import Alert from '@mui/material/Alert'
import Dialog from '@mui/material/Dialog'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import UpdateTriagePanel from './UpdateTriagePanel'

export default function UpsertTriageModal({
  regressionIds,
  triage,
  setComplete,
  buttonText,
  submissionDelay = 0,
  fields = [],
}) {
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

  const [triageEntryData, setTriageEntryData] = React.useState({
    url: '',
    type: 'type',
    description: '',
  })

  // The modal stays mounted across a same-record reload, so the form has to
  // follow that prop or a save would submit the previous copy.
  useEffect(() => {
    if (triage !== undefined) {
      setTriageEntryData(triage)
    }
  }, [triage])

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
              <TriageDialogContent>
                <LoadingContainer>
                  <CircularProgress aria-label="Loading triage information..." />
                  <Typography>Loading triage information...</Typography>
                </LoadingContainer>
              </TriageDialogContent>
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
