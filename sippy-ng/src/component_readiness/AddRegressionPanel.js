import { Button, MenuItem, Select, Tab, Tabs } from '@mui/material'
import { getTriagesAPIUrl, jiraUrlPrefix } from './CompReadyUtils'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TriageFields from './TriageFields'

export default function AddRegressionPanel({
  triages,
  regressionId,
  existingTriageId,
  setExistingTriageId,
  setAlertText,
  setAlertSeverity,
  handleNewTriageFormCompletion,
  completeTriageSubmission,
  triageEntryData,
  setTriageEntryData,
}) {
  const [tabIndex, setTabIndex] = React.useState(0)
  const handleTabChange = (event, newValue) => {
    setTabIndex(newValue)
  }

  const handleAddToExistingTriageSubmit = () => {
    const existingTriage = triages.find((t) => t.id === existingTriageId)

    const updatedTriage = {
      ...existingTriage,
      regressions: [...existingTriage.regressions, { id: regressionId }],
    }

    fetch(getTriagesAPIUrl(existingTriage.id), {
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
        setExistingTriageId(triages[0].id) //reset the form to the first element
      }
      completeTriageSubmission()
    })
  }

  const handleExistingTriageChange = (event) => {
    setExistingTriageId(event.target.value)
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
              value={existingTriageId}
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
              setAlertText={setAlertText}
              setAlertSeverity={setAlertSeverity}
              triageEntryData={triageEntryData}
              handleFormCompletion={handleNewTriageFormCompletion}
              setTriageEntryData={setTriageEntryData}
              submitButtonText={'Create Entry'}
            />
          </Fragment>
        )}
      </DialogContent>
    </Fragment>
  )
}

AddRegressionPanel.propTypes = {
  triages: PropTypes.array.isRequired,
  regressionId: PropTypes.number.isRequired,
  existingTriageId: PropTypes.number.isRequired,
  setExistingTriageId: PropTypes.func.isRequired,
  triageEntryData: PropTypes.object.isRequired,
  setTriageEntryData: PropTypes.func.isRequired,
  setAlertText: PropTypes.func.isRequired,
  setAlertSeverity: PropTypes.func.isRequired,
  handleNewTriageFormCompletion: PropTypes.func.isRequired,
  completeTriageSubmission: PropTypes.func.isRequired,
}
