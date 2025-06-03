import { Box, Button, Grid, Tab, Tabs, Typography } from '@mui/material'
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

function RegressedTestsTabPanel(props) {
  const { children, activeIndex, index, ...other } = props

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

export default function RegressedTestsModal(props) {
  const [activeTabIndex, setActiveTabIndex] = React.useState(0)

  const handleTabChange = (event, newValue) => {
    setActiveTabIndex(newValue)
  }

  const triageEntriesExist = props.triageEntries.length > 0

  return (
    <Fragment>
      <Dialog
        fullWidth={true}
        maxWidth={false}
        open={props.isOpen}
        onClose={props.close}
      >
        <Grid className="regressed-tests-dialog">
          <Tabs
            value={activeTabIndex}
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
          <RegressedTestsTabPanel activeIndex={activeTabIndex} index={0}>
            <RegressedTestsPanel
              regressedTests={props.unresolvedTests}
              setTriageEntryCreated={props.setTriageEntryCreated}
              filterVals={props.filterVals}
            />
          </RegressedTestsTabPanel>
          <RegressedTestsTabPanel activeIndex={activeTabIndex} index={1}>
            <RegressedTestsPanel
              regressedTests={props.regressedTests}
              setTriageEntryCreated={props.setTriageEntryCreated}
              filterVals={props.filterVals}
            />
          </RegressedTestsTabPanel>
          {!triageEntriesExist && (
            <RegressedTestsTabPanel activeIndex={activeTabIndex} index={2}>
              <TriagedIncidentsPanel
                triagedIncidents={props.triagedIncidents}
              />
            </RegressedTestsTabPanel>
          )}
          {triageEntriesExist && (
            <RegressedTestsTabPanel activeIndex={activeTabIndex} index={2}>
              <TriagedTestsPanel
                triageEntries={props.triageEntries}
                allRegressedTests={props.allRegressedTests}
                filterVals={props.filterVals}
              />
            </RegressedTestsTabPanel>
          )}
          <RegressedTestsTabPanel activeIndex={activeTabIndex} index={3}>
            <RegressedTestsPanel
              regressedTests={props.allRegressedTests}
              setTriageEntryCreated={props.setTriageEntryCreated}
              filterVals={props.filterVals}
            />
          </RegressedTestsTabPanel>
          <Button
            style={{ marginTop: 20, marginBottom: 20, marginLeft: 20 }}
            variant="contained"
            color="primary"
            onClick={props.close}
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
