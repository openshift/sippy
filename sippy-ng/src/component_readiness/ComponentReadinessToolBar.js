import './ComponentReadiness.css'
import {
  AppBar,
  Badge,
  Box,
  FormControlLabel,
  Popover,
  Tooltip,
} from '@mui/material'
import {
  BooleanParam,
  NumberParam,
  StringParam,
  useQueryParam,
  useQueryParams,
} from 'use-query-params'
import {
  BugReport,
  Clear,
  GridView,
  HelpCenter,
  InsertLink,
  LocalHospital,
  Refresh,
  ViewColumn,
  Widgets,
} from '@mui/icons-material'
import { CapabilitiesContext } from '../App'
import { CompReadyVarsContext } from './CompReadyVars'
import {
  formColumnName,
  generateTestDetailsReportLink,
  getTriagesAPIUrl,
  mergeRegressionData,
  Search,
  SearchIconWrapper,
  StyledInputBase,
} from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { useGlobalChat } from '../chat/useGlobalChat'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useEffect } from 'react'
import RegressedTestsModal from './RegressedTestsModal'
import SwitchControl from '@mui/material/Switch'
import Toolbar from '@mui/material/Toolbar'

export default function ComponentReadinessToolBar(props) {
  const {
    searchRowRegex,
    handleSearchRowRegexChange,
    searchColumnRegex,
    handleSearchColumnRegexChange,
    redOnlyChecked,
    handleRedOnlyCheckboxChange,
    clearSearches,
    data,
    setTriageActionTaken,
    filterVals,
  } = props

  const [regressedTests, setRegressedTests] = React.useState([])
  const [allRegressedTests, setAllRegressedTests] = React.useState([])
  const [unresolvedTests, setUnresolvedTests] = React.useState([])
  const [triageEntries, setTriageEntries] = React.useState([])
  const [isLoaded, setIsLoaded] = React.useState(false)
  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const localDBEnabled = capabilitiesContext.includes('local_db')
  const varsContext = useContext(CompReadyVarsContext)
  const { updatePageContext } = useGlobalChat()

  React.useEffect(() => {
    // triage entries will only be available when there is a postgres connection
    let triageFetch
    if (localDBEnabled) {
      triageFetch = fetch(getTriagesAPIUrl(), {
        method: 'GET',
      }).then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
    } else {
      triageFetch = Promise.resolve([])
    }

    triageFetch.then((triages) => {
      const merged = mergeRegressionData(data, triages)
      setRegressedTests(merged.untriagedRegressedTests)
      setAllRegressedTests(merged.allRegressions)
      setUnresolvedTests(merged.unresolvedRegressedTests)
      const activeRegressionIds = merged.allRegressions?.map(
        (test) => test.regression?.id
      )
      let triagesAssociatedWithActiveRegressions = []
      triages.forEach((triage) => {
        // Filter out any included regressions that are no longer active
        triage.regressions = triage.regressions.filter((regression) =>
          activeRegressionIds.includes(regression.id)
        )
        // If there are no regressions left, the triage record will be hidden
        if (triage.regressions.length > 0) {
          triagesAssociatedWithActiveRegressions.push(triage)
        }
      })
      setTriageEntries(triagesAssociatedWithActiveRegressions)
      setIsLoaded(true)
    })
  }, [])

  // Update page context when regression data is loaded
  useEffect(() => {
    if (!isLoaded || !varsContext.sampleRelease) {
      return
    }

    // Helper to format test data for context
    const formatTestForContext = (test) => ({
      component: test.component,
      capability: test.capability,
      test_name: test.test_name,
      test_id: test.test_id,
      test_suite: test.test_suite,
      variants: formColumnName({ variants: test.variants }),
      status: test.status,
      regression_id: test.regression?.id,
      regressed_since: test.regression?.opened,
      last_failure: test.last_failure,
      details_link: generateTestDetailsReportLink(
        test,
        filterVals,
        varsContext.expandEnvironment
      ),
    })

    updatePageContext({
      page: 'component-readiness',
      url: window.location.href,

      instructions: `This is the Component Readiness main view showing a matrix of components vs environments with regression status. 
        Focus on unresolved and untriaged regressions - these are the most important items requiring attention.
        When providing test detail links, use the details_link field provided for each test.
        Status values: negative numbers indicate regressions (more negative = more severe), positive numbers indicate stability.
        Unresolved regressions have status <= -200 (not yet hopefully fixed).`,

      suggestedQuestions: [
        'What regressions are new today?',
        "What regressions haven't been triaged?",
      ],

      data: {
        sample_release: varsContext.sampleRelease,
        base_release: varsContext.baseRelease,
        view: varsContext.view,

        summary: {
          total_unresolved: unresolvedTests.length,
          total_untriaged: regressedTests.length,
          total_all_regressions: allRegressedTests.length,
        },

        // Unresolved regressions (status <= -200) - most critical
        unresolved_regressions: unresolvedTests
          .slice(0, 30)
          .map(formatTestForContext),

        // Untriaged regressions - need attention
        untriaged_regressions: regressedTests
          .slice(0, 30)
          .map(formatTestForContext),
      },
    })

    // Clear context on unmount
    return () => {
      updatePageContext(null)
    }
  }, [
    isLoaded,
    regressedTests,
    allRegressedTests,
    unresolvedTests,
    varsContext.sampleRelease,
    varsContext.baseRelease,
    varsContext.view,
    filterVals,
    varsContext.expandEnvironment,
    updatePageContext,
  ])

  const linkToReport = () => {
    const currentUrl = new URL(window.location.href)
    if (searchRowRegex && searchRowRegex !== '') {
      currentUrl.searchParams.set('searchRow', searchRowRegex)
    }

    if (searchColumnRegex && searchColumnRegex !== '') {
      currentUrl.searchParams.set('searchColumn', searchColumnRegex)
    }

    if (redOnlyChecked) {
      currentUrl.searchParams.set('redOnly', '1')
    }

    return currentUrl.href
  }

  const copyLinkToReport = (event) => {
    event.preventDefault()
    navigator.clipboard.writeText(linkToReport())
    setCopyPopoverEl(event.currentTarget)
    setTimeout(() => setCopyPopoverEl(null), 2000)
  }

  const [copyPopoverEl, setCopyPopoverEl] = React.useState(null)
  const copyPopoverOpen = Boolean(copyPopoverEl)

  const [regressedTestDialog = false, setRegressedTestDialog] = useQueryParam(
    'regressedModal',
    BooleanParam
  )
  const [, setQuery] = useQueryParams(
    {
      regressedModalTab: NumberParam,
      regressedModalRow: StringParam,
      regressedModalPage: NumberParam,
      regressedModalTestRow: NumberParam,
      regressedModalTestPage: NumberParam,
    },
    { updateType: 'replaceIn' }
  )

  const closeRegressedTestsDialog = () => {
    setRegressedTestDialog(undefined, 'replaceIn')
    setQuery(
      {
        regressedModalTab: undefined,
        regressedModalRow: undefined,
        regressedModalPage: undefined,
        regressedModalTestRow: undefined,
        regressedModalTestPage: undefined,
      },
      'replaceIn'
    )
  }

  if (!isLoaded) {
    return <div>Loading...</div>
  }

  return (
    <div>
      <Box sx={{ flexGrow: 1 }}>
        <AppBar elevation={1} position="static">
          <Toolbar sx={{ leftPadding: 0 }}>
            {handleSearchRowRegexChange ? (
              <Search>
                <SearchIconWrapper>
                  <Widgets />
                </SearchIconWrapper>
                <StyledInputBase
                  placeholder="Search Row"
                  inputProps={{ 'aria-label': 'search' }}
                  value={searchRowRegex}
                  onChange={handleSearchRowRegexChange}
                />
              </Search>
            ) : (
              <></>
            )}
            {handleSearchColumnRegexChange ? (
              <Search>
                <SearchIconWrapper>
                  <ViewColumn />
                </SearchIconWrapper>
                <StyledInputBase
                  placeholder="Search Column"
                  inputProps={{ 'aria-label': 'search' }}
                  value={searchColumnRegex}
                  onChange={handleSearchColumnRegexChange}
                />
              </Search>
            ) : (
              <></>
            )}
            {handleRedOnlyCheckboxChange ? (
              <Box display="flex" alignItems="center" sx={{ paddingBottom: 2 }}>
                <FormControlLabel
                  control={
                    <SwitchControl
                      checked={redOnlyChecked}
                      onChange={handleRedOnlyCheckboxChange}
                      color="primary"
                      size="small"
                      style={{ borderRadius: 1 }}
                    />
                  }
                  htmlFor="redOnlyCheckbox"
                  style={{
                    textAlign: 'left',
                    marginTop: 15,
                  }}
                  label="Red Only"
                ></FormControlLabel>
              </Box>
            ) : (
              <></>
            )}

            {handleSearchColumnRegexChange ||
            handleRedOnlyCheckboxChange ||
            handleSearchRowRegexChange ? (
              <Fragment>
                <IconButton
                  size="large"
                  aria-label="Copy Link"
                  color="inherit"
                  onClick={copyLinkToReport}
                >
                  <Tooltip title="Copy link to search">
                    <InsertLink />
                  </Tooltip>
                </IconButton>
                <IconButton
                  size="large"
                  aria-label="Clear Search"
                  color="inherit"
                  onClick={clearSearches}
                >
                  <Tooltip title="Clear searches">
                    <Clear />
                  </Tooltip>
                </IconButton>
              </Fragment>
            ) : (
              <></>
            )}

            <Box sx={{ flexGrow: 1 }} />
            {props.forceRefresh ? (
              <Box sx={{ display: { md: 'flex' } }}>
                <IconButton
                  size="large"
                  aria-label="Force data refresh"
                  color="inherit"
                  onClick={props.forceRefresh}
                >
                  <Tooltip title="Force data refresh">
                    <Refresh />
                  </Tooltip>
                </IconButton>
              </Box>
            ) : (
              <></>
            )}
            <Box sx={{ display: { md: 'flex' } }}>
              <IconButton
                size="large"
                aria-label="Show open bugs"
                color="inherit"
                href="https://issues.redhat.com/issues/?filter=12432468"
              >
                <Tooltip title="Show open bugs">
                  <BugReport />
                </Tooltip>
              </IconButton>
            </Box>

            {localDBEnabled && (
              <Box sx={{ display: { md: 'flex' } }}>
                <IconButton
                  size="large"
                  aria-label="Show all triage records"
                  color="inherit"
                  href="/sippy-ng/component_readiness/triages"
                >
                  <Tooltip title="Show all triage records">
                    <LocalHospital />
                  </Tooltip>
                </IconButton>
              </Box>
            )}

            <Box sx={{ display: { md: 'flex' } }}>
              <IconButton
                size="large"
                aria-label="Show Regressed Tests"
                color="inherit"
                onClick={() => setRegressedTestDialog(true, 'replaceIn')}
              >
                <Badge badgeContent={unresolvedTests.length} color="error">
                  <Tooltip title="Show regressed tests">
                    <GridView />
                  </Tooltip>
                </Badge>
              </IconButton>
            </Box>
            <Box sx={{ display: { md: 'flex' } }}>
              <IconButton
                size="large"
                aria-label="Help"
                color="inherit"
                component={Link}
                to="/component_readiness/help"
              >
                <Tooltip title="Help and FAQ">
                  <HelpCenter />
                </Tooltip>
              </IconButton>
            </Box>
          </Toolbar>
        </AppBar>
      </Box>

      <Popover
        id="copyPopover"
        open={copyPopoverOpen}
        anchorEl={copyPopoverEl}
        onClose={() => setCopyPopoverEl(null)}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'center',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'center',
        }}
      >
        Link copied!
      </Popover>
      <RegressedTestsModal
        regressedTests={regressedTests}
        allRegressedTests={allRegressedTests}
        unresolvedTests={unresolvedTests}
        triageEntries={triageEntries}
        setTriageActionTaken={setTriageActionTaken}
        filterVals={filterVals}
        isOpen={regressedTestDialog}
        close={closeRegressedTestsDialog}
      />
    </div>
  )
}

ComponentReadinessToolBar.propTypes = {
  searchRowRegex: PropTypes.string,
  handleSearchRowRegexChange: PropTypes.func,
  searchColumnRegex: PropTypes.string,
  handleSearchColumnRegexChange: PropTypes.func,
  redOnlyChecked: PropTypes.bool,
  handleRedOnlyCheckboxChange: PropTypes.func,
  clearSearches: PropTypes.func,
  data: PropTypes.object,
  forceRefresh: PropTypes.func,
  setTriageActionTaken: PropTypes.func,
  filterVals: PropTypes.string.isRequired,
}
