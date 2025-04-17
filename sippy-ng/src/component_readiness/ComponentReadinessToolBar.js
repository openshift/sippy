import './ComponentReadiness.css'
import {
  AppBar,
  Badge,
  Box,
  FormControlLabel,
  Popover,
  Tooltip,
} from '@mui/material'
import { BooleanParam, useQueryParam } from 'use-query-params'
import {
  BugReport,
  Clear,
  GridView,
  HelpCenter,
  InsertLink,
  Refresh,
  ViewColumn,
  Widgets,
} from '@mui/icons-material'
import {
  getTriagesAPIUrl,
  mergeRegressionData,
  Search,
  SearchIconWrapper,
  StyledInputBase,
} from './CompReadyUtils'
import { Link } from 'react-router-dom'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
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
    filterVals,
  } = props

  const [triageEntryCreated, setTriageEntryCreated] = React.useState(false)
  const [regressedTests, setRegressedTests] = React.useState([])
  const [allRegressedTests, setAllRegressedTests] = React.useState([])
  const [triagedIncidents, setTriagedIncidents] = React.useState([])
  const [triageEntries, setTriageEntries] = React.useState([])
  const [isLoaded, setIsLoaded] = React.useState(false)
  React.useEffect(() => {
    fetch(getTriagesAPIUrl(), {
      method: 'GET',
    })
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
      .then((triages) => {
        setTriageEntries(triages)
        const merged = mergeRegressionData(data, triages)
        setRegressedTests(merged.length > 0 ? merged[0] : null)
        setAllRegressedTests(merged.length > 1 ? merged[1] : null)
        setTriagedIncidents(merged.length > 2 ? merged[2] : null)
        setTriageEntryCreated(false)
        setIsLoaded(true)
      })
  }, [triageEntryCreated])

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
  const closeRegressedTestsDialog = () => {
    setRegressedTestDialog(false, 'replaceIn')
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

            <Box sx={{ display: { md: 'flex' } }}>
              <IconButton
                size="large"
                aria-label="Show Regressed Tests"
                color="inherit"
                onClick={() => setRegressedTestDialog(true, 'replaceIn')}
              >
                <Badge badgeContent={allRegressedTests.length} color="error">
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
        triagedIncidents={triagedIncidents}
        triageEntries={triageEntries}
        setTriageEntryCreated={setTriageEntryCreated}
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
  filterVals: PropTypes.string.isRequired,
}
