import {
  Button,
  DialogActions,
  MenuItem,
  Select,
  Snackbar,
  Tab,
  Tabs,
} from '@mui/material'
import { getTriagesAPIUrl, jiraUrlPrefix } from './CompReadyUtils'
import Alert from '@mui/material/Alert'
import Dialog from '@mui/material/Dialog'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TriageFields from './TriageFields'

export default function UpsertTriageModal({
  regressionId,
  setHasBeenTriaged,
  buttonText,
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
          setExistingTriageID(filtered[0].id)
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

  const [existingTriageID, setExistingTriageID] = React.useState(0)
  const handleExistingTriageChange = (event) => {
    setExistingTriageID(event.target.value)
  }

  const handleAddToExistingTriageSubmit = () => {
    const existingTriage = triages.find((t) => t.id === existingTriageID)

    const updatedTriage = {
      ...existingTriage,
      regressions: [...existingTriage.regressions, { id: regressionId }],
    }

    fetch(getTriagesAPIUrl() + '/' + existingTriage.id, {
      method: 'PUT',
      body: JSON.stringify(updatedTriage),
    }).then((response) => {
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

      setAlertText(
        'successfully added test to triage record: ' +
          formatTriageURLDescription(existingTriage)
      )
      setAlertSeverity('success')
      if (triages.length > 0) {
        setExistingTriageID(triages[0].id) //reset the form to the first element
      }
      completeTriageSubmission()
    })
  }

  const [newTriageEntryData, setNewTriageEntryData] = React.useState({
    url: '',
    type: 'type',
    description: '',
  })

  const handleNewTriageFormCompletion = () => {
    setNewTriageEntryData({
      url: '',
      type: 'type',
      description: '',
    })
    setTriageModalOpen(false)
    completeTriageSubmission()
  }

  const completeTriageSubmission = () => {
    setTriageModalOpen(false)
    // allow a couple seconds to view the success message before the page gets reloaded
    const timer = setTimeout(() => {
      setHasBeenTriaged(true)
    }, 2000)
    return () => clearTimeout(timer)
  }

  const [tabIndex, setTabIndex] = React.useState(0)
  const handleTabChange = (event, newValue) => {
    setTabIndex(newValue)
  }

  const [alertText, setAlertText] = React.useState('')
  const [alertSeverity, setAlertSeverity] = React.useState('success')
  const handleAlertClose = (event, reason) => {
    if (reason === 'clickaway') {
      return
    }
    setAlertText('')
    setAlertSeverity('success')
  }

  const formatTriageURLDescription = (triage) => {
    let url = triage.url
    if (url.startsWith(jiraUrlPrefix)) {
      url = url.slice(jiraUrlPrefix.length)
    }
    return url + ' - ' + triage.description
  }

  const addToExisting = tabIndex === 0
  const addToNew = tabIndex === 1

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
        <DialogTitle>Add Triage</DialogTitle>
        <DialogContent>
          <Tabs
            value={tabIndex}
            onChange={handleTabChange}
            indicatorColor="secondary"
            textColor="primary"
            variant="fullWidth"
          >
            <Tab label="Existing Triage" />
            <Tab label="New Triage" />
          </Tabs>
          {addToExisting && (
            <Fragment>
              <h3>Add to existing Triage</h3>
              <Select
                id="existing-triage"
                name="existing-triage"
                label="Existing Triage"
                value={existingTriageID}
                onChange={handleExistingTriageChange}
              >
                {triages.map((triageEntry, index) => (
                  <MenuItem key={index} value={triageEntry.id}>
                    {formatTriageURLDescription(triageEntry)}
                  </MenuItem>
                ))}
              </Select>
              <Button
                variant="contained"
                color="primary"
                sx={{ margin: '0 10px' }}
                onClick={handleAddToExistingTriageSubmit}
              >
                Add to Entry
              </Button>
            </Fragment>
          )}
          {addToNew && (
            <Fragment>
              <h3>Add to new Triage</h3>
              <TriageFields
                regressionId={regressionId}
                setAlertText={setAlertText}
                setAlertSeverity={setAlertSeverity}
                triageEntryData={newTriageEntryData}
                handleFormCompletion={handleNewTriageFormCompletion}
                setTriageEntryData={setNewTriageEntryData}
              />
            </Fragment>
          )}
        </DialogContent>
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
  setHasBeenTriaged: PropTypes.func.isRequired,
  buttonText: PropTypes.string.isRequired,
}
