import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns'
import { DateTimePicker, LocalizationProvider } from '@mui/x-date-pickers'
import {
  FormHelperText,
  MenuItem,
  Select,
  TextField,
  Tooltip,
} from '@mui/material'
import { getTriagesAPIUrl, jiraUrlPrefix } from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
import Button from '@mui/material/Button'
import ExistingTriageSelector from './ExistingTriageSelector'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles({
  triageForm: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '10px 0',
  },
  formFields: {
    display: 'flex',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 16,
  },
  validationErrors: {
    color: 'red',
  },
})

export default function TriageFields({
  triageId,
  triages,
  setAlertText,
  setAlertSeverity,
  triageEntryData,
  setTriageEntryData,
  handleFormCompletion,
  submitButtonText,
  existingTriageId,
  setExistingTriageId,
  handleAddToExistingTriage,
}) {
  const classes = useStyles()

  const [matchingTriages, setMatchingTriages] = React.useState([])
  const [triageValidationErrors, setTriageValidationErrors] = React.useState([])

  const updating = triageId > 0

  const handleTriageChange = (e) => {
    const { name, value } = e.target

    setTriageEntryData((prevData) => {
      const updatedData = {
        ...prevData,
        [name]: value,
      }

      if (!updating && name === 'url') {
        setMatchingTriages(
          triages.filter((triage) => triage.url === updatedData.url)
        )
      }

      return updatedData
    })
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
      if (matchingTriages.length > 0) {
        const confirmed = window.confirm(
          'There are existing triage entries with the same URL. Are you sure you want to create a new entry instead of adding to an existing one?'
        )
        if (!confirmed) {
          return
        }
      }

      let data
      let triagesAPIUrl
      let method
      if (updating) {
        data = triageEntryData
        triagesAPIUrl = getTriagesAPIUrl(triageId)
        method = 'PUT'
      } else {
        triagesAPIUrl = getTriagesAPIUrl()
        method = 'POST'
        data = {
          url: triageEntryData.url,
          type: triageEntryData.type,
          description: triageEntryData.description,
        }

        if (managingIds) {
          data.regressions = triageEntryData.ids.map((id) => {
            return { id: Number(id) }
          })
        }
      }

      fetch(triagesAPIUrl, {
        method: method,
        body: JSON.stringify(data),
      }).then((response) => {
        if (!response.ok) {
          response.json().then((data) => {
            let errorMessage = 'invalid response returned from server'
            if (data?.code) {
              errorMessage =
                'error ' +
                (updating ? 'updating' : 'creating') +
                ' triage entry: ' +
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

        if (updating) {
          setAlertText('successfully updated triage entry')
        } else {
          setAlertText('successfully created triage entry')
        }
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
      <div className={classes.formFields}>
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
        {updating && (
          <LocalizationProvider dateAdapter={AdapterDateFns}>
            <DateTimePicker
              label="Resolution Date"
              value={
                triageEntryData.resolved?.Valid
                  ? triageEntryData.resolved?.Time
                  : null
              }
              onChange={(date) =>
                setTriageEntryData((prevData) => ({
                  ...prevData,
                  resolved: { Time: date, Valid: date !== null },
                }))
              }
              renderInput={(props) => (
                <TextField variant="standard" {...props} />
              )}
            />
          </LocalizationProvider>
        )}
        <Button
          variant="contained"
          color="primary"
          onClick={handleTriageEntrySubmit}
        >
          {submitButtonText}
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
      {matchingTriages.length > 0 && (
        <div>
          <h4>
            Triage Entries with Matching Jira Exist
            <Tooltip title="It is likely unwanted to create a new triage entry for the same Jira. Please select an existing triage entry to add to instead.">
              <InfoIcon
                fontSize="small"
                style={{ marginLeft: 8, verticalAlign: 'middle' }}
              />
            </Tooltip>
          </h4>
          <ExistingTriageSelector
            triages={matchingTriages}
            existingTriageId={existingTriageId}
            setExistingTriageId={setExistingTriageId}
            onSubmit={handleAddToExistingTriage}
          />
        </div>
      )}
    </div>
  )
}

TriageFields.propTypes = {
  triageId: PropTypes.number,
  triages: PropTypes.array.isRequired,
  setAlertText: PropTypes.func.isRequired,
  setAlertSeverity: PropTypes.func.isRequired,
  triageEntryData: PropTypes.object.isRequired,
  setTriageEntryData: PropTypes.func.isRequired,
  handleFormCompletion: PropTypes.func.isRequired,
  submitButtonText: PropTypes.string.isRequired,
  // used when the user opts to add to existing triage with matching url
  existingTriageId: PropTypes.number.isRequired,
  setExistingTriageId: PropTypes.func.isRequired,
  handleAddToExistingTriage: PropTypes.func.isRequired,
}
