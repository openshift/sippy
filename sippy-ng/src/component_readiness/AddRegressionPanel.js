import { getTriagesAPIUrl } from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
import { Tab, Tabs } from '@mui/material'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import ExistingTriageSelector from './ExistingTriageSelector'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import RegressionPotentialMatchesTab from './RegressionPotentialMatchesTab'
import TriageFields from './TriageFields'

const useStyles = makeStyles((theme) => ({
  noMatchesMessage: {
    textAlign: 'center',
    padding: '8px',
    fontStyle: 'italic',
    backgroundColor: '#f5f5f5',
    borderRadius: '4px',
    margin: '8px 0',
  },
}))

export default function AddRegressionPanel({
  triages,
  regressionIds,
  setAlertText,
  setAlertSeverity,
  handleNewTriageFormCompletion,
  completeTriageSubmission,
  triageEntryData,
  setTriageEntryData,
  commonClasses,
}) {
  const localClasses = useStyles()
  const classes = { ...commonClasses, ...localClasses }
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
  const [existingTriageId, setExistingTriageId] =
    React.useState(defaultTriageId)

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
        setExistingTriageId(defaultTriageId) //reset the form to the first element
      }
      completeTriageSubmission()
    })
  }

  const showPotentialMatches =
    hasSingleRegression && tabIndex === 0 && hasMatches
  const addToExisting = tabIndex === (hasSingleRegression ? 1 : 0)
  const addToNew = tabIndex === (hasSingleRegression ? 2 : 1)

  return (
    <Fragment>
      <DialogTitle>Triage</DialogTitle>
      <DialogContent className={classes.dialogContent}>
        <Tabs
          value={tabIndex}
          onChange={handleTabChange}
          indicatorColor="secondary"
          textColor="primary"
          variant="fullWidth"
          className={classes.tabsContainer}
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
        </Tabs>
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
            <h3 className={classes.centeredHeading}>Add to existing Triage</h3>
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
            <h3 className={classes.centeredHeading}>Create new Triage</h3>
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
  commonClasses: PropTypes.object.isRequired,
}
