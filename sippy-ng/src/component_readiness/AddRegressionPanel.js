import { getTriagesAPIUrl } from './CompReadyUtils'
import { Tab, Tabs } from '@mui/material'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import ExistingTriageSelector from './ExistingTriageSelector'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TriageFields from './TriageFields'

export default function AddRegressionPanel({
  triages,
  regressionIds,
  setAlertText,
  setAlertSeverity,
  handleNewTriageFormCompletion,
  completeTriageSubmission,
  triageEntryData,
  setTriageEntryData,
}) {
  triages.sort((a, b) => new Date(b.updated_at) - new Date(a.updated_at))

  const [tabIndex, setTabIndex] = React.useState(0)
  const handleTabChange = (event, newValue) => {
    setTabIndex(newValue)
  }

  const [existingTriageId, setExistingTriageId] = React.useState(
    triages.length > 0 ? triages[0].id : null
  )

  const handleAddToExistingTriageSubmit = () => {
    const existingTriage = triages.find(
      (triage) => triage.id === existingTriageId
    )
    const updatedTriage = {
      ...existingTriage,
      regressions: [
        ...existingTriage.regressions,
        ...regressionIds.map((id) => {
          return { id: Number(id) }
        }),
      ],
    }

    fetch(getTriagesAPIUrl(existingTriageId), {
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
        'successfully added test to triage record: ' + existingTriage.url
      )
      setAlertSeverity('success')
      if (triages.length > 0) {
        setExistingTriageId(triages[0].id) //reset the form to the first element
      }
      completeTriageSubmission()
    })
  }

  const addToExisting = tabIndex === 0
  const addToNew = tabIndex === 1

  return (
    <Fragment>
      <DialogTitle>Triage</DialogTitle>
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
            <ExistingTriageSelector
              triages={triages}
              existingTriageId={existingTriageId}
              setExistingTriageId={setExistingTriageId}
              onSubmit={handleAddToExistingTriageSubmit}
            />
          </Fragment>
        )}
        {addToNew && (
          <Fragment>
            <h3>Create new Triage</h3>
            <TriageFields
              triages={triages}
              setAlertText={setAlertText}
              setAlertSeverity={setAlertSeverity}
              triageEntryData={triageEntryData}
              handleFormCompletion={handleNewTriageFormCompletion}
              setTriageEntryData={setTriageEntryData}
              submitButtonText={'Create Entry'}
              existingTriageId={existingTriageId}
              setExistingTriageId={setExistingTriageId}
              handleAddToExistingTriage={handleAddToExistingTriageSubmit}
            />
          </Fragment>
        )}
      </DialogContent>
    </Fragment>
  )
}

AddRegressionPanel.propTypes = {
  triages: PropTypes.array.isRequired,
  regressionIds: PropTypes.array.isRequired,
  triageEntryData: PropTypes.object.isRequired,
  setTriageEntryData: PropTypes.func.isRequired,
  setAlertText: PropTypes.func.isRequired,
  setAlertSeverity: PropTypes.func.isRequired,
  handleNewTriageFormCompletion: PropTypes.func.isRequired,
  completeTriageSubmission: PropTypes.func.isRequired,
}
