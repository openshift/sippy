import {
  ArrayParam,
  NumberParam,
  StringParam,
  useQueryParam,
  useQueryParams,
} from 'use-query-params'
import { Box, Button, Grid, Tab, Tabs, Typography } from '@mui/material'
import Dialog from '@mui/material/Dialog'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import RegressedTestsPanel from './RegressedTestsPanel'
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
  triageEntries,
  setTriageActionTaken,
  filterVals,
  isOpen,
  close,
}) {
  const [activeTab = 0, setActiveTab] = useQueryParam(
    'regressedModalTab',
    NumberParam,
    { updateType: 'replaceIn' }
  )
  const [componentFilter = [], setComponentFilter] = useQueryParam(
    'component',
    ArrayParam,
    { updateType: 'replaceIn' }
  )
  const [, setQuery] = useQueryParams(
    {
      regressedModalRow: StringParam,
      regressedModalPage: NumberParam,
      regressedModalTestRow: NumberParam,
      regressedModalTestPage: NumberParam,
      regressedModalFilters: StringParam,
      regressedModalTestFilters: StringParam,
    },
    { updateType: 'replaceIn' }
  )

  const handleTabChange = (event, newValue) => {
    setActiveTab(newValue)
    // Reset pagination and selection when switching tabs, but keep filters
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

  // Filter tests by component if component filter is specified
  const filterTestsByComponent = (tests) => {
    if (!componentFilter || componentFilter.length === 0) {
      return tests
    }
    return tests.filter((test) => componentFilter.includes(test.component))
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
            <Tab
              label="Triaged"
              disabled={!triageEntriesExist}
              {...tabProps(2)}
            />
            <Tab label="All" {...tabProps(3)} />
          </Tabs>
          <RegressedTestsTabPanel activeIndex={activeTab} index={0}>
            <RegressedTestsPanel
              regressedTests={filterTestsByComponent(unresolvedTests)}
              setTriageActionTaken={setTriageActionTaken}
              filterVals={filterVals}
            />
          </RegressedTestsTabPanel>
          <RegressedTestsTabPanel activeIndex={activeTab} index={1}>
            <RegressedTestsPanel
              regressedTests={filterTestsByComponent(regressedTests)}
              setTriageActionTaken={setTriageActionTaken}
              filterVals={filterVals}
            />
          </RegressedTestsTabPanel>
          {triageEntriesExist && (
            <RegressedTestsTabPanel activeIndex={activeTab} index={2}>
              <TriagedTestsPanel
                triageEntries={triageEntries}
                allRegressedTests={filterTestsByComponent(allRegressedTests)}
                filterVals={filterVals}
              />
            </RegressedTestsTabPanel>
          )}
          <RegressedTestsTabPanel activeIndex={activeTab} index={3}>
            <RegressedTestsPanel
              regressedTests={filterTestsByComponent(allRegressedTests)}
              setTriageActionTaken={setTriageActionTaken}
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
  triageEntries: PropTypes.array,
  setTriageActionTaken: PropTypes.func,
  filterVals: PropTypes.string.isRequired,
  isOpen: PropTypes.bool,
  close: PropTypes.func,
}
