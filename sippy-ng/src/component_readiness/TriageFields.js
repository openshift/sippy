import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns'
import { CompReadyVarsContext } from './CompReadyVars'
import { DateTimePicker, LocalizationProvider } from '@mui/x-date-pickers'
import {
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
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
import React, { useContext } from 'react'

const useStyles = makeStyles({
  triageForm: {
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    padding: '8px 0',
  },
  formFields: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'stretch',
    gap: 12,
    width: '100%',
  },
  validationErrors: {
    color: 'red',
  },
  submitButton: {
    alignSelf: 'flex-start',
    minWidth: '120px',
  },
  infoIcon: {
    marginLeft: 8,
    verticalAlign: 'middle',
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
  const { view } = useContext(CompReadyVarsContext)

  const [matchingTriages, setMatchingTriages] = React.useState([])
  const [triageValidationErrors, setTriageValidationErrors] = React.useState([])
  const [potentialMatchesDialog, setPotentialMatchesDialog] = React.useState({
    open: false,
    count: 0,
    triageId: null,
  })

  const updating = triageId > 0

  const handlePotentialMatchesYes = () => {
    const triageDetailsUrl = `/sippy-ng/triages/${potentialMatchesDialog.triageId}?openMatches=1`
    window.open(triageDetailsUrl, '_blank', 'noopener,noreferrer')
    setPotentialMatchesDialog({ open: false, count: 0, triageId: null })
    setAlertText('successfully created triage entry')
    setAlertSeverity('success')
    handleFormCompletion()
  }

  const handlePotentialMatchesNo = () => {
    setPotentialMatchesDialog({ open: false, count: 0, triageId: null })
    setAlertText('successfully created triage entry')
    setAlertSeverity('success')
    handleFormCompletion()
  }

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
          setAlertSeverity('success')
          handleFormCompletion()
        } else {
          response.json().then((createdTriage) => {
            fetch(`${getTriagesAPIUrl(createdTriage.id)}/matches?view=${view}`)
              .then((matchesResponse) => {
                if (matchesResponse.status === 200) {
                  return matchesResponse.json()
                }
                return []
              })
              .then((potentialMatches) => {
                if (potentialMatches && potentialMatches.length > 0) {
                  setPotentialMatchesDialog({
                    open: true,
                    count: potentialMatches.length,
                    triageId: createdTriage.id,
                  })
                } else {
                  setAlertText('successfully created triage entry')
                  setAlertSeverity('success')
                  handleFormCompletion()
                }
              })
              .catch((error) => {
                console.error('Error fetching potential matches:', error)
                // If fetching matches fails, proceed with normal flow
                setAlertText('successfully created triage entry')
                setAlertSeverity('success')
                handleFormCompletion()
              })
          })
        }
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
          fullWidth
        />
        <TextField
          name="description"
          label="Description"
          value={triageEntryData.description}
          onChange={handleTriageChange}
          fullWidth
        />
        <Select
          name="type"
          label="Type"
          value={triageEntryData.type}
          onChange={handleTriageChange}
          fullWidth
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
                <TextField variant="standard" fullWidth {...props} />
              )}
            />
          </LocalizationProvider>
        )}
        <Button
          variant="contained"
          color="primary"
          onClick={handleTriageEntrySubmit}
          className={classes.submitButton}
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
              <InfoIcon fontSize="small" className={classes.infoIcon} />
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

      <Dialog
        open={potentialMatchesDialog.open}
        onClose={handlePotentialMatchesNo}
        aria-labelledby="potential-matches-dialog-title"
        aria-describedby="potential-matches-dialog-description"
      >
        <DialogTitle id="potential-matches-dialog-title">
          Potential Matching Regressions Found
        </DialogTitle>
        <DialogContent>
          <DialogContentText id="potential-matches-dialog-description">
            We found {potentialMatchesDialog.count} potentially matching
            regression{potentialMatchesDialog.count !== 1 ? 's' : ''} for this
            triage. Would you like to view them, and potentially link them to
            this triage?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handlePotentialMatchesNo} color="secondary">
            No, Continue
          </Button>
          <Button
            onClick={handlePotentialMatchesYes}
            color="primary"
            variant="contained"
          >
            Yes, View Matches
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  )
}

TriageFields.propTypes = {
  triageId: PropTypes.number,
  triages: PropTypes.array,
  setAlertText: PropTypes.func.isRequired,
  setAlertSeverity: PropTypes.func.isRequired,
  triageEntryData: PropTypes.object.isRequired,
  setTriageEntryData: PropTypes.func.isRequired,
  handleFormCompletion: PropTypes.func.isRequired,
  submitButtonText: PropTypes.string.isRequired,
  // used when the user opts to add to existing triage with matching url
  existingTriageId: PropTypes.number,
  setExistingTriageId: PropTypes.func,
  handleAddToExistingTriage: PropTypes.func,
}
