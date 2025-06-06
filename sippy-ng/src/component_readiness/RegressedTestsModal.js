import { Box, Button, Grid, Tab, Tabs, Typography } from '@mui/material'
import {
  NumberParam,
  StringParam,
  useQueryParam,
  useQueryParams,
} from 'use-query-params'
import Dialog from '@mui/material/Dialog'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import RegressedTestsPanel from './RegressedTestsPanel'
import TriagedIncidentsPanel from './TriagedIncidentsPanel'
import TriagedTestsPanel from './TriagedTestsPanel'

function tabProps(index) {
  return {
    id: `regressions-tab-${index}`,
    'aria-controls': `regressions-tabpanel-${index}`,
  }
}

RegressedTestsTabPanel.propTypes = {
  children: PropTypes.node,
  index: PropTypes.number.isRequired,
  activeIndex: PropTypes.number.isRequired,
}

function RegressedTestsTabPanel({ children, activeIndex, index, ...other }) {
  return (
    <div
      role="tabpanel"
      hidden={activeIndex !== index}
      id={`regressions-tabpanel-${index}`}
      aria-labelledby={`regressions-tab-${index}`}
      {...other}
    >
      {activeIndex === index && (
        <Box sx={{ p: 3 }}>
          <Typography>{children}</Typography>
        </Box>
      )}
    </div>
  )
}

export default function RegressedTestsModal({
  regressedTests,
  allRegressedTests,
  unresolvedTests,
  triagedIncidents,
  triageEntries,
  setTriageEntryCreated,
  filterVals,
  isOpen,
  close,
}) {
  const [activeTab = 0, setActiveTab] = useQueryParam(
    'regressedModalTab',
    NumberParam,
    { updateType: 'replaceIn' }
  )
  const [, setQuery] = useQueryParams(
    {
      regressedModalRow: StringParam,
      regressedModalPage: NumberParam,
      regressedModalTestRow: NumberParam,
      regressedModalTestPage: NumberParam,
    },
    { updateType: 'replaceIn' }
  )

  const handleTabChange = (event, newValue) => {
    setActiveTab(newValue)
    // The active pages and rows in the DataGrid are most likely no longer relevant when switching tabs
    setQuery(
      {
        regressedModalRow: undefined,
        regressedModalPage: undefined,
        regressedModalTestRow: undefined,
        regressedModalTestPage: undefined,
      },
      'replaceIn'
    )
  }

  const triageEntriesExist = triageEntries.length > 0

  return (
    <Fragment>
      <Dialog fullWidth={true} maxWidth={false} open={isOpen} onClose={close}>
        <Grid className="regressed-tests-dialog">
          <Tabs
            value={activeTab}
            onChange={handleTabChange}
            aria-label="Regressed Tests Tabs"
          >
            <Tab label="Unresolved" {...tabProps(0)} />
            <Tab label="Untriaged" {...tabProps(1)} />
            {triageEntriesExist && <Tab label="Triaged" {...tabProps(2)} />}
            {!triageEntriesExist && (
              <Tab label="Triaged Incidents" {...tabProps(2)} />
            )}
            <Tab label="All" {...tabProps(3)} />
          </Tabs>
          <RegressedTestsTabPanel activeIndex={activeTab} index={0}>
            <RegressedTestsPanel
              regressedTests={unresolvedTests}
              setTriageEntryCreated={setTriageEntryCreated}
              filterVals={filterVals}
            />
          </RegressedTestsTabPanel>
          <RegressedTestsTabPanel activeIndex={activeTab} index={1}>
            <RegressedTestsPanel
              regressedTests={regressedTests}
              setTriageEntryCreated={setTriageEntryCreated}
              filterVals={filterVals}
            />
          </RegressedTestsTabPanel>
          {!triageEntriesExist && (
            <RegressedTestsTabPanel activeIndex={activeTab} index={2}>
              <TriagedIncidentsPanel triagedIncidents={triagedIncidents} />
            </RegressedTestsTabPanel>
          )}
          {triageEntriesExist && (
            <RegressedTestsTabPanel activeIndex={activeTab} index={2}>
              <TriagedTestsPanel
                triageEntries={triageEntries}
                allRegressedTests={allRegressedTests}
                filterVals={filterVals}
              />
            </RegressedTestsTabPanel>
          )}
          <RegressedTestsTabPanel activeIndex={activeTab} index={3}>
            <RegressedTestsPanel
              regressedTests={allRegressedTests}
              setTriageEntryCreated={setTriageEntryCreated}
              filterVals={filterVals}
            />
          </RegressedTestsTabPanel>
          <Button
            style={{ marginTop: 20, marginBottom: 20, marginLeft: 20 }}
            variant="contained"
            color="primary"
            onClick={close}
          >
            CLOSE
          </Button>
        </Grid>
      </Dialog>
    </Fragment>
  )
}

RegressedTestsModal.propTypes = {
  regressedTests: PropTypes.array,
  allRegressedTests: PropTypes.array,
  unresolvedTests: PropTypes.array,
  triagedIncidents: PropTypes.array,
  triageEntries: PropTypes.array,
  setTriageEntryCreated: PropTypes.func,
  filterVals: PropTypes.string.isRequired,
  isOpen: PropTypes.bool,
  close: PropTypes.func,
}
