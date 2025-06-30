import { Button } from '@mui/material'
import { jiraUrlPrefix } from './CompReadyUtils'
import Autocomplete from '@mui/lab/Autocomplete'
import PropTypes from 'prop-types'
import React from 'react'
import TextField from '@mui/material/TextField'

export default function ExistingTriageSelector({
  triages,
  existingTriageId,
  setExistingTriageId,
  onSubmit,
}) {
  const handleExistingTriageChange = (event, newValue) => {
    setExistingTriageId(newValue.id)
  }

  const formatTriageURLDescription = (triage) => {
    let url = triage.url
    if (url.startsWith(jiraUrlPrefix)) {
      url = url.slice(jiraUrlPrefix.length)
    }
    return url + ' - ' + triage.description
  }

  return (
    <>
      <Autocomplete
        id="existing-triage"
        name="existing-triage"
        options={triages}
        value={triages.find((t) => t.id === existingTriageId)}
        getOptionLabel={(triage) => {
          return formatTriageURLDescription(triage)
        }}
        isOptionEqualToValue={(triage, value) => triage.id === value?.id}
        renderInput={(params) => (
          <TextField {...params} label="Existing Triage" />
        )}
        onChange={handleExistingTriageChange}
      />

      <Button
        variant="contained"
        color="primary"
        sx={{ margin: '10px 0' }}
        onClick={onSubmit}
      >
        Add to Entry
      </Button>
    </>
  )
}

ExistingTriageSelector.propTypes = {
  triages: PropTypes.array.isRequired,
  existingTriageId: PropTypes.number.isRequired,
  setExistingTriageId: PropTypes.func.isRequired,
  onSubmit: PropTypes.func.isRequired,
}
