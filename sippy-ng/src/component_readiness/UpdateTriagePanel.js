import { Button, FormHelperText, Tab, Tabs } from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { formColumnName, getTriagesAPIUrl } from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
import { relativeTime } from '../helpers'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TriageFields from './TriageFields'

const useStyles = makeStyles((theme) => ({
  regressionList: {
    listStyle: 'none',
  },
  removeButton: {
    marginTop: theme.spacing(2),
  },
  errorText: {
    color: 'red',
  },
  dataGrid: {
    marginBottom: theme.spacing(2),
    '& .MuiDataGrid-columnHeaders': {
      backgroundColor: 'rgba(0, 0, 0, 0.04)',
    },
  },
}))

export default function UpdateTriagePanel({
  triage,
  triageEntryData,
  setTriageEntryData,
  setAlertText,
  setAlertSeverity,
  handleTriageFormCompletion,
  commonClasses,
}) {
  const localClasses = useStyles()
  const classes = { ...commonClasses, ...localClasses }
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
      <DialogContent className={classes.dialogContent}>
        <Tabs
          value={tabIndex}
          onChange={handleTabChange}
          indicatorColor="secondary"
          textColor="primary"
          variant="fullWidth"
          className={classes.tabsContainer}
        >
          <Tab label="Update Fields" />
          <Tab
            label="Remove Regressions"
            disabled={triage.regressions.length < 2}
          />
        </Tabs>

        {updateFields && (
          <Fragment>
            <h3 className={classes.centeredHeading}>Update Information</h3>
            <TriageFields
              triageId={triage.id}
              setAlertText={setAlertText}
              setAlertSeverity={setAlertSeverity}
              triageEntryData={triageEntryData}
              setTriageEntryData={setTriageEntryData}
              handleFormCompletion={handleTriageFormCompletion}
              submitButtonText={'Update'}
            />
          </Fragment>
        )}
        {updateRegressions && (
          <Fragment>
            <h3 className={classes.centeredHeading}>Remove Regressions</h3>
            <DataGrid
              rows={triage.regressions}
              columns={[
                {
                  field: 'select',
                  headerName: 'Remove',
                  flex: 6,
                  sortable: false,
                  filterable: false,
                  disableColumnMenu: true,
                  renderCell: (params) => (
                    <input
                      type="checkbox"
                      name="regression"
                      value={params.row.id}
                      onChange={handleRegressionChange}
                      checked={removedRegressions.includes(
                        String(params.row.id)
                      )}
                    />
                  ),
                },
                {
                  field: 'test_name',
                  headerName: 'Test Name',
                  flex: 50,
                  renderCell: (param) => (
                    <div className="test-name">{param.value}</div>
                  ),
                },
                {
                  field: 'variants',
                  headerName: 'Variants',
                  flex: 35,
                  valueGetter: (params) => {
                    if (
                      params.row.variants &&
                      typeof params.row.variants === 'object'
                    ) {
                      return formColumnName({ variants: params.row.variants })
                    }
                    return params.row.variants || ''
                  },
                  renderCell: (param) => (
                    <div className="test-name">{param.value}</div>
                  ),
                },
                {
                  field: 'opened',
                  headerName: 'Regressed Since',
                  flex: 12,
                  valueGetter: (params) => {
                    if (!params.row.opened) {
                      return ''
                    }
                    const regressedSinceDate = new Date(params.row.opened)
                    return relativeTime(regressedSinceDate, new Date())
                  },
                  renderCell: (param) => (
                    <div className="regressed-since">{param.value}</div>
                  ),
                },
              ]}
              getRowId={(row) => row.id}
              autoHeight={true}
              getRowHeight={() => 80}
              disableSelectionOnClick
              hideFooter={triage.regressions.length <= 10}
              pageSize={10}
              rowsPerPageOptions={[10, 25, 50]}
              disableColumnMenu={true}
              className={classes.dataGrid}
            />
            {validationMessage !== '' && (
              <FormHelperText className={classes.errorText}>
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
  commonClasses: PropTypes.object.isRequired,
}
