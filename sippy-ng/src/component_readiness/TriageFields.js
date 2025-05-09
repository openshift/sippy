import { FormHelperText, MenuItem, Select, TextField } from '@mui/material'
import { getTriagesAPIUrl, jiraUrlPrefix } from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
import Button from '@mui/material/Button'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles({
  triageForm: {
    display: 'flex',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 16,
    padding: '10px 0',
  },
  validationErrors: {
    color: 'red',
  },
})

export default function TriageFields({
  regressionId,
  setAlertText,
  setAlertSeverity,
  triageEntryData,
  setTriageEntryData,
  handleFormCompletion,
}) {
  const classes = useStyles()

  const [triageValidationErrors, setTriageValidationErrors] = React.useState([])

  const handleTriageChange = (e) => {
    const { name, value } = e.target

    setTriageEntryData((prevData) => ({
      ...prevData,
      [name]: value,
    }))
  }

  const handleTriageEntrySubmit = () => {
    const managingIds = triageEntryData.hasOwnProperty('ids')
    const validationErrors = []
    if (triageEntryData.type === 'type') {
      validationErrors.push('invalid type, please make a selection')
    }
    if (!triageEntryData.url.startsWith(jiraUrlPrefix)) {
      validationErrors.push('invalid url, should begin with ' + jiraUrlPrefix)
    }
    if (triageEntryData.description.length < 1) {
      validationErrors.push('invalid description, cannot be blank')
    }
    if (managingIds && triageEntryData.ids.length < 1) {
      validationErrors.push('no tests selected, please select at least one')
    }
    setTriageValidationErrors(validationErrors)

    if (validationErrors.length === 0) {
      const data = {
        url: triageEntryData.url,
        type: triageEntryData.type,
        description: triageEntryData.description,
      }
      if (managingIds) {
        data.regressions = triageEntryData.ids.map((id) => {
          return { id: Number(id) }
        })
      } else if (regressionId > 0) {
        data.regressions = [{ id: regressionId }]
      }

      fetch(getTriagesAPIUrl(), {
        method: 'POST',
        body: JSON.stringify(data),
      }).then((response) => {
        if (!response.ok) {
          response.json().then((data) => {
            let errorMessage = 'invalid response returned from server'
            if (data?.code) {
              errorMessage =
                'error creating triage entry: ' +
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

        setAlertText('successfully created triage entry')
        setAlertSeverity('success')
        handleFormCompletion()
      })
    }
  }

  const triageTypeOptions = [
    'type',
    'ci-infra',
    'product-infra',
    'product',
    'test',
  ]

  return (
    <div className={classes.triageForm}>
      <TextField
        name="url"
        label="Jira URL"
        value={triageEntryData.url}
        onChange={handleTriageChange}
      />
      <TextField
        name="description"
        label="Description"
        value={triageEntryData.description}
        onChange={handleTriageChange}
      />
      <Select
        name="type"
        label="Type"
        value={triageEntryData.type}
        onChange={handleTriageChange}
      >
        {triageTypeOptions.map((option, index) => (
          <MenuItem key={index} value={option}>
            {option}
          </MenuItem>
        ))}
      </Select>
      <Button
        variant="contained"
        color="primary"
        onClick={handleTriageEntrySubmit}
      >
        Create Entry
      </Button>
      {triageValidationErrors && (
        <FormHelperText className={classes.validationErrors}>
          {triageValidationErrors.map((text, index) => (
            <span key={index}>
              {text}
              <br />
            </span>
          ))}
        </FormHelperText>
      )}
    </div>
  )
}

TriageFields.propTypes = {
  regressionId: PropTypes.number,
  setAlertText: PropTypes.func.isRequired,
  setAlertSeverity: PropTypes.func.isRequired,
  triageEntryData: PropTypes.object.isRequired,
  setTriageEntryData: PropTypes.func.isRequired,
  handleFormCompletion: PropTypes.func.isRequired,
}
