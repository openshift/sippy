import { Button, DialogActions, Snackbar, Tooltip } from '@mui/material'
import { getTriagesAPIUrl } from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
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
}))

export default function UpsertTriageModal({
  regressionIds,
  triage,
  setComplete,
  buttonText,
  submissionDelay = 0,
}) {
  const commonClasses = useCommonTriageStyles()
  const regressionAddMode =
    regressionIds !== undefined && regressionIds.length > 0
  const triageDetailsUpdateMode = triage !== undefined

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
        if (regressionAddMode) {
          triages = triages.filter(
            (triage) =>
              !triage.regressions.some((regression) =>
                regressionIds.includes(regression.id)
              )
          )
        }
        setTriages(triages)
        setTriageModalOpen(true)
      })
      .catch((error) => {
        setAlertText('Error retrieving existing triage records')
        setAlertSeverity('error')
        console.error(error)
      })
  }
  const handleTriageModalClosed = () => {
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
          {triageDetailsUpdateMode && (
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
          {regressionAddMode && (
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
}
