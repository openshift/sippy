import { Button, FormHelperText, Tab, Tabs } from '@mui/material'
import { getTriagesAPIUrl } from './CompReadyUtils'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TriageFields from './TriageFields'

export default function UpdateTriagePanel({
  triage,
  triageEntryData,
  setTriageEntryData,
  setAlertText,
  setAlertSeverity,
  handleTriageFormCompletion,
}) {
  const [removedRegressions, setRemovedRegressions] = React.useState([])
  const [validationMessage, setValidationMessage] = React.useState('')

  const handleRegressionChange = (e) => {
    setValidationMessage('')
    const { value, checked } = e.target
    if (checked) {
      if (removedRegressions.length + 1 === triage.regressions.length) {
        setValidationMessage(
          'Cannot remove all regressions, must leave at least one associated with the triage record'
        )
        return
      }
      setRemovedRegressions((prev) => [...prev, value])
    } else {
      setRemovedRegressions((prev) =>
        prev.filter((existingRegressionId) => existingRegressionId !== value)
      )
    }
  }

  const submitRegressionRemoval = () => {
    if (removedRegressions.length === 0) {
      setValidationMessage(
        'No regressions selected, please select at least one to remove'
      )
      return
    }

    const updatedRegressions = triage.regressions.filter(
      (regression) => !removedRegressions.includes(String(regression.id))
    )

    fetch(getTriagesAPIUrl(triage.id), {
      method: 'PUT',
      body: JSON.stringify({ ...triage, regressions: updatedRegressions }),
    }).then((response) => {
      if (!response.ok) {
        response.json().then((data) => {
          let errorMessage = 'invalid response returned from server'
          if (data?.code) {
            errorMessage =
              'error removing regressions from triage entry: ' +
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
        'successfully removed ' +
          removedRegressions.length +
          ' regression(s) from triage record'
      )
      setAlertSeverity('success')
      handleTriageFormCompletion()
    })
  }

  const [tabIndex, setTabIndex] = React.useState(0)
  const handleTabChange = (event, newValue) => {
    setTabIndex(newValue)
  }
  const updateFields = tabIndex === 0
  const updateRegressions = tabIndex === 1
  return (
    <Fragment>
      <DialogTitle>Update Triage</DialogTitle>
      <DialogContent>
        <Tabs
          value={tabIndex}
          onChange={handleTabChange}
          indicatorColor="secondary"
          textColor="primary"
          variant="fullWidth"
        >
          <Tab label="Update Fields" />
          <Tab
            label="Remove Regressions"
            disabled={triage.regressions.length < 2}
          />
        </Tabs>

        {updateFields && (
          <TriageFields
            triageId={triage.id}
            setAlertText={setAlertText}
            setAlertSeverity={setAlertSeverity}
            triageEntryData={triageEntryData}
            setTriageEntryData={setTriageEntryData}
            handleFormCompletion={handleTriageFormCompletion}
            submitButtonText={'Update'}
          />
        )}
        {updateRegressions && (
          <Fragment>
            <ul style={{ listStyle: 'none' }}>
              {triage.regressions.map((regression) => {
                return (
                  <li key={regression.id}>
                    <input
                      type="checkbox"
                      name="regression"
                      value={regression.id}
                      onChange={handleRegressionChange}
                      checked={removedRegressions.includes(
                        String(regression.id)
                      )}
                    />{' '}
                    {regression.test_name}
                    <hr />
                  </li>
                )
              })}
            </ul>
            {validationMessage !== '' && (
              <FormHelperText sx={{ color: 'red' }}>
                {validationMessage}
              </FormHelperText>
            )}
            <Button variant="contained" onClick={submitRegressionRemoval}>
              Remove Selected
            </Button>
          </Fragment>
        )}
      </DialogContent>
    </Fragment>
  )
}

UpdateTriagePanel.propTypes = {
  triage: PropTypes.object.isRequired,
  triageEntryData: PropTypes.object.isRequired,
  setTriageEntryData: PropTypes.func.isRequired,
  setAlertText: PropTypes.func.isRequired,
  setAlertSeverity: PropTypes.func.isRequired,
  handleTriageFormCompletion: PropTypes.func.isRequired,
}
