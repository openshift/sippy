import { getTriagesAPIUrl } from './CompReadyUtils'
import { Tab, Typography } from '@mui/material'
import { TriageDialogContent, TriageTabs } from './TriageStyledComponents'
import DialogTitle from '@mui/material/DialogTitle'
import ExistingTriageSelector from './ExistingTriageSelector'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import RegressionPotentialMatchesTab from './RegressionPotentialMatchesTab'
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

  // We can only find potential matching triages when we are triaging a single regression at a time
  const hasSingleRegression = regressionIds.length === 1
  const [hasMatches, setHasMatches] = React.useState(true)
  // Start with potential matches if single regression, otherwise existing triage
  const [tabIndex, setTabIndex] = React.useState(0)

  const handleTabChange = (event, newValue) => {
    // Only prevent switching to potential matches tab if it's disabled or doesn't exist
    if (newValue === 0 && !hasMatches) {
      return
    }
    setTabIndex(newValue)
  }

  // We only want to enable the "Potential Matches" tab when we actually have matches
  const handleMatchingTriagesFetched = (foundMatches) => {
    setHasMatches(foundMatches)
    if (!foundMatches) {
      setTabIndex(1) // Switch to existing triage tab if no matches found
    }
  }

  const defaultTriageId = triages.length > 0 ? triages[0].id : null
  const [existingTriageId, setExistingTriageId] = React.useState(
    triages.length > 0 ? triages[0].id : null
  )

  const handleAddToExistingTriageSubmit = async () => {
    try {
      const triageUrl = getTriagesAPIUrl(existingTriageId)
      // List entries are sparse. Retrieve the full record before replacing it.
      const detailsResponse = await fetch(triageUrl)
      if (!detailsResponse.ok) {
        throw new Error('error retrieving triage entry for update')
      }
      const existingTriage = await detailsResponse.json()
      // Compact lists can include already-linked triages; duplicate IDs would fail validation.
      const combinedRegressionIds = new Set([
        ...existingTriage.regressions.map((regression) => regression.id),
        ...regressionIds.map(Number),
      ])
      const updatedTriage = {
        ...existingTriage,
        regressions: Array.from(combinedRegressionIds, (id) => ({ id })),
      }

      const response = await fetch(triageUrl, {
        method: 'PUT',
        body: JSON.stringify(updatedTriage),
      })
      if (!response.ok) {
        const data = await response.json().catch(() => null)
        const errorMessage = data?.code
          ? 'error adding test to triage entry: ' +
            data.code +
            ': ' +
            data.message
          : 'invalid response returned from server'
        throw new Error(errorMessage)
      }

      setAlertText(
        'successfully added test to triage record: ' + existingTriage.url
      )
      setAlertSeverity('success')
      if (triages.length > 0) {
        setExistingTriageId(defaultTriageId) //reset the form to the first element
      }
      completeTriageSubmission()
    } catch (error) {
      console.error(error)
      setAlertText(error.message)
      setAlertSeverity('error')
    }
  }

  const showPotentialMatches =
    hasSingleRegression && tabIndex === 0 && hasMatches
  const addToExisting = tabIndex === (hasSingleRegression ? 1 : 0)
  const addToNew = tabIndex === (hasSingleRegression ? 2 : 1)

  return (
    <Fragment>
      <DialogTitle>Triage</DialogTitle>
      <TriageDialogContent>
        <TriageTabs
          value={tabIndex}
          onChange={handleTabChange}
          indicatorColor="secondary"
          textColor="primary"
          variant="fullWidth"
        >
          {hasSingleRegression && (
            <Tab
              label={
                hasMatches
                  ? 'Potential Matches'
                  : 'Potential Matches (none found)'
              }
              disabled={!hasMatches}
            />
          )}
          <Tab label="Existing Triage" />
          <Tab label="New Triage" />
        </TriageTabs>
        {showPotentialMatches && (
          <RegressionPotentialMatchesTab
            regressionId={regressionIds[0]}
            setAlertText={setAlertText}
            setAlertSeverity={setAlertSeverity}
            completeTriageSubmission={completeTriageSubmission}
            onMatchesFound={handleMatchingTriagesFetched}
          />
        )}
        {addToExisting && (
          <Fragment>
            <Typography variant="h6" sx={{ textAlign: 'center' }}>
              Add to existing Triage
            </Typography>
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
            <Typography variant="h6" sx={{ textAlign: 'center' }}>
              Create new Triage
            </Typography>
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
      </TriageDialogContent>
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
