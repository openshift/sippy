import { Button, DialogActions, Snackbar } from '@mui/material'
import { getTriagesAPIUrl } from './CompReadyUtils'
import AddRegressionPanel from './AddRegressionPanel'
import Alert from '@mui/material/Alert'
import Dialog from '@mui/material/Dialog'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import UpdateTriagePanel from './UpdateTriagePanel'

export default function UpsertTriageModal({
  regressionId,
  triage,
  setComplete,
  buttonText,
  submissionDelay = 0,
}) {
  const [triages, setTriages] = React.useState([])
  const [triageModalOpen, setTriageModalOpen] = React.useState(false)
  const handleTriageModalOpen = () => {
    // Only get all existing entries when actually adding/editing a triage
    fetch(getTriagesAPIUrl())
      .then((response) => {
        if (response.status !== 200) {
          throw new Error(
            `API call failed ${getTriagesAPIUrl()} Returned + ${
              response.status
            }`
          )
        }
        return response.json()
      })
      .then((triages) => {
        const filtered = triages.filter(
          (triage) =>
            !triage.regressions.some(
              (regression) => regression.id === regressionId
            )
        )
        setTriages(filtered)
        if (filtered.length > 0) {
          setExistingTriageId(filtered[0].id)
        }
      })
      .catch((error) => {
        setAlertText('Error retrieving existing triage records')
        setAlertSeverity('error')
        console.error(error)
      })
    setTriageModalOpen(true)
  }
  const handleTriageModalClosed = () => {
    setTriageModalOpen(false)
  }

  const [existingTriageId, setExistingTriageId] = React.useState(0)

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
    ids: [regressionId],
  }
  if (triage !== undefined) {
    initialTriage = triage
  }
  const [triageEntryData, setTriageEntryData] = React.useState(initialTriage)

  const [alertText, setAlertText] = React.useState('')
  const [alertSeverity, setAlertSeverity] = React.useState('success')
  const handleAlertClose = (event, reason) => {
    if (reason === 'clickaway') {
      return
    }
    setAlertText('')
    setAlertSeverity('success')
  }

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
      <Button
        sx={{ margin: '10px 0' }}
        variant="contained"
        onClick={handleTriageModalOpen}
      >
        {buttonText}
      </Button>
      <Dialog
        fullWidth
        maxWidth="md"
        open={triageModalOpen}
        onClose={handleTriageModalClosed}
      >
        {triage !== undefined && (
          <UpdateTriagePanel
            triage={triage}
            setAlertText={setAlertText}
            setAlertSeverity={setAlertSeverity}
            triageEntryData={triageEntryData}
            handleTriageFormCompletion={handleTriageFormCompletion}
            setTriageEntryData={setTriageEntryData}
          />
        )}
        {regressionId > 0 && (
          <AddRegressionPanel
            triages={triages}
            regressionId={regressionId}
            existingTriageId={existingTriageId}
            setExistingTriageId={setExistingTriageId}
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
    </Fragment>
  )
}

UpsertTriageModal.propTypes = {
  regressionId: PropTypes.number,
  triage: PropTypes.object,
  setComplete: PropTypes.func.isRequired,
  buttonText: PropTypes.string.isRequired,
  submissionDelay: PropTypes.number,
}
