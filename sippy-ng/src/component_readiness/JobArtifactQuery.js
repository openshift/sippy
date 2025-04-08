import {
  Autocomplete,
  Button,
  Checkbox,
  CircularProgress,
  FormControl,
  FormControlLabel,
  FormHelperText,
  InputLabel,
  Link,
  MenuItem,
  Popover,
  Select,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
} from '@mui/material'
import {
  Close,
  FileCopy,
  Link as LinkIcon,
  OpenInNew,
} from '@mui/icons-material'
import { getArtifactQueryAPIUrl } from './CompReadyUtils'
import { relativeTime, safeEncodeURIComponent } from '../helpers'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

const emptyContentMatch = {
  type: 'none',
  none: {},
  string: { match: '', limit: 12, before: 0, after: 0 },
  regex: { match: '', limit: 12, before: 0, after: 0 },
}
const prefilledOptions = new Map([
  ['Reset query parameters', {}],
  [
    'OpenShift install logs (files only)',
    {
      fileMatch:
        'artifacts/*e2e*/*-install-install*/artifacts/.openshift_install-*.log',
      type: 'string',
    },
  ],
  [
    'node journals (files only)',
    {
      fileMatch: 'artifacts/*e2e*/gather-extra/artifacts/nodes/*/journal',
      type: 'string',
    },
  ],
  [
    'audit logs (files only)',
    {
      fileMatch: 'artifacts/*e2e*/gather-extra/artifacts/audit_logs/*/*.log',
      type: 'string',
    },
  ],
  [
    'junits (files only)',
    {
      fileMatch: 'artifacts/**/{junit,monitor,e2e-monitor}*.xml',
      type: 'string',
    },
  ],
  [
    'error logs in build-log.txt',
    {
      fileMatch: 'build-log.txt',
      type: 'regex',
      regex: {
        match: '\\[error\\]|"error"|level=error',
        limit: 12,
        before: 1,
        after: 0,
      },
    },
  ],
  [
    'LimitExceeded in install logs',
    {
      fileMatch:
        'artifacts/*e2e*/*-install-install*/artifacts/.openshift_install-*.log',
      type: 'regex',
      regex: {
        match: 'level=error.*LimitExceeded',
        limit: 12,
        before: 1,
        after: 1,
      },
    },
  ],
])

export default function JobArtifactQuery(props) {
  const { searchJobRunIds, jobRunsLookup, handleToggleJAQOpen } = props

  /*********************************************************************************
   shared state for artifact query components
   *********************************************************************************/

  // The file match string that will be used to filter the artifacts
  const [fileMatch, setFileMatch] = React.useState('')
  // The content match parameters to apply against the matched artifacts
  const [contentMatch, setContentMatch] = React.useState(emptyContentMatch)

  // This reservoir holds the current artifact search results indexed by run ID.
  // TODO: make this actually cache data for at least some queries that have already been made
  // New data for job run(s) will be inserted as they are selected and searches performed.
  // The rows given to the table are constructed from these according to the visible job run IDs.
  const jobRunSearches = React.useRef(new Map())
  const [apiCallURL, setApiCallURL] = React.useState('')
  // give a visual indicator while loading data
  const [loading, setLoading] = React.useState(false)

  // filters for what to display in the table
  const [displayOnlySelected, setDisplayOnlySelected] = React.useState(false)
  const [displayOnlyArtifacts, setDisplayOnlyArtifacts] = React.useState(false)
  const [displayOnlyMatches, setDisplayOnlyMatches] = React.useState(false)
  // used to track which runs are selected from the table for further action
  const [selectedJobRunIds, setSelectedJobRunIds] = React.useState(new Set())
  // TODO: enable sorting the table by column headers
  const [sortModel, setSortModel] = React.useState([
    { field: 'job_run_id', sort: 'desc' },
  ])
  // the search result rows for our table, raw and filtered
  const emptyRows = searchJobRunIds
    .keys()
    .map((jobRunId) => jobRunsLookup.get(jobRunId))
    .toArray()
  const [rows, setRows] = React.useState(emptyRows)
  const [filteredRows, setFilteredRows] = React.useState([])

  /*********************************************************************************
   effects that react to state changes
   *********************************************************************************/

  // make the API call to get the artifact matches based on UI parameters changing
  React.useEffect(() => {
    // reset loading state when the query changes, even to empty
    jobRunSearches.current.clear()
    setLoading(false)

    // enable canceling this search when no longer needed
    const abortController = new AbortController()

    // set up the artifact query when parameters are given
    const [qParams, url] = constructArtifactQuery()
    setApiCallURL(url)
    if (!url) {
      setRows(emptyRows)
      return
    }
    setLoading(true)

    fetch(url, { signal: abortController.signal })
      .then((response) => {
        setLoading(false)
        if (response.status < 200 || response.status >= 300) {
          throw new Error(
            `Return code = ${response.status} (${response.statusText})`
          )
        }
        return response.json()
      })
      .then((json) => {
        if (json.job_runs) {
          for (const run of json.job_runs) {
            jobRunSearches.current.set(run.id, run)
          }
          setRows(
            searchJobRunIds
              .keys()
              .map((jobRunId) => {
                return {
                  ...jobRunsLookup.get(jobRunId),
                  searchResult: jobRunSearches.current.get(jobRunId),
                }
              })
              .toArray()
          )
        }
      })
      .catch((error) => {
        if (error.name === 'AbortError') {
          console.log(`request aborted: ${url}`)
          // don't want to clear Loading in the case where next query already started
        } else {
          // setFetchError(`API call failed: ${url}\n${error}`)
          console.log(`API call failed: ${url}`, error)
          setLoading(false)
        }
      })

    // return cleanup function to abort the request when the component unmounts or a new query is made
    return () => {
      abortController.abort()
    }
  }, [fileMatch, contentMatch])

  // filter the rows and content to be displayed
  React.useEffect(() => {
    let filtered = []
    for (const jobRun of rows) {
      if (displayOnlySelected && !selectedJobRunIds.has(jobRun.job_run_id))
        continue
      if (displayOnlyArtifacts && !jobRun.searchResult?.artifacts) continue
      if (!displayOnlyMatches) {
        filtered.push(jobRun)
        continue
      }
      // displaying only artifacts with content matches; filter out artifacts and job runs without them
      let filteredArtifacts = []
      for (const artifact of jobRun.searchResult?.artifacts || []) {
        if ((artifact.matched_content?.matches || []).length > 0) {
          filteredArtifacts.push(artifact)
        }
      }
      if (displayOnlyArtifacts && filteredArtifacts.length === 0) continue // no artifact matches in this job run
      filtered.push({
        ...jobRun,
        searchResult: {
          ...(jobRun.searchResult || {}),
          artifacts: filteredArtifacts,
        },
      })
    }
    setFilteredRows(filtered)
  }, [
    rows,
    selectedJobRunIds,
    displayOnlySelected,
    displayOnlyArtifacts,
    displayOnlyMatches,
  ])

  /*********************************************************************************
   functions and local React components that use the shared state
   *********************************************************************************/

  function constructArtifactQuery() {
    if (!searchJobRunIds.size || !fileMatch || fileMatch.length < 3) {
      return [null, null]
    }
    let prowJobRuns = Array.from(searchJobRunIds.keys()).join(',')
    let params = { pathGlob: fileMatch }
    let type = contentMatch.type
    if (['string', 'regex'].includes(type)) {
      params[type === 'string' ? 'textContains' : 'textRegex'] =
        contentMatch[type].match
      params.beforeContext = contentMatch[type].before
      params.afterContext = contentMatch[type].after
      params.maxFileMatches = contentMatch[type].limit
    }
    let url = `${getArtifactQueryAPIUrl()}?prowJobRuns=${prowJobRuns}`
    Object.keys(params).forEach((key) => {
      url += '&' + key + '=' + safeEncodeURIComponent(params[key])
    })
    return [params, url]
  }

  function JAQPrefilled() {
    const [prefilled, setPrefilled] = React.useState('')
    function handlePrefilledChange(e) {
      const params = prefilledOptions.get(e.target.value)
      // little weird but we don't actually want the choice to persist in the select: setPrefilled(e.target.value)
      setPrefilled('')
      if (params) {
        setFileMatch(params.fileMatch)
        setContentMatch({
          ...emptyContentMatch,
          ...params,
        })
      }
    }

    return (
      <Fragment>
        <FormControl>
          <InputLabel id="prefilledLabel">Common queries</InputLabel>
          <Select
            variant="standard"
            labelId="prefilledLabel"
            label="Common queries"
            autoWidth={true}
            size="small"
            sx={{ minWidth: '20em' }}
            value={prefilled}
            onChange={handlePrefilledChange}
          >
            {[
              ...prefilledOptions.keys().map((option, index) => (
                <MenuItem key={index} value={option}>
                  {option}
                </MenuItem>
              )),
            ]}
          </Select>
        </FormControl>
      </Fragment>
    )
  }

  // component to choose the match for artifact file names
  function JAQFileMatch() {
    const handleFileMatchChange = (event, newValue) => {
      newValue = newValue === null ? '' : newValue
      if (newValue !== '' && newValue.length < 3) {
        alert('Artifact path glob must be at least 3 characters long')
      } else {
        setFileMatch(newValue)
      }
    }

    return (
      <Fragment>
        <FormControl>
          <Autocomplete
            freeSolo={true}
            defaultValue=""
            size="small"
            options={[
              'build-log.txt',
              'prowjob.json',
              'artifacts/ci-operator.log',
              'artifacts/*e2e*/gather-extra/build-log.txt',
              'artifacts/*e2e*/gather-extra/artifacts/audit_logs/*/*.log',
              'artifacts/*e2e*/gather-extra/artifacts/nodes/*/journal',
              'artifacts/*e2e*/openshift-e2e-test/artifacts/junit/e2e-events*.json',
              'artifacts/**/{junit,monitor,e2e-monitor}*.xml',
              'artifacts/*e2e*/*-install-install*/artifacts/.openshift_install-*.log',
            ]}
            renderInput={(params) => (
              <TextField {...params} label="Artifact path glob" />
            )}
            value={fileMatch}
            onChange={handleFileMatchChange}
          />
          <FormHelperText>
            Enter a file name pattern relative to the top of the artifact path
            (see examples in pull-down) to match against artifacts in the job
            run. &nbsp;
            <Link
              href="https://cloud.google.com/storage/docs/json_api/v1/objects/list#list-object-glob"
              target="_blank"
              rel="noopener noreferrer"
            >
              Wildcards may be used per the glob pattern syntax.
            </Link>
            &nbsp; Results are limited to 12 files per job run.
          </FormHelperText>
        </FormControl>
      </Fragment>
    )
  }

  // components for choosing how to match the content of the artifacts
  function JAQContentMatch() {
    function handleMatchTypeChange(e) {
      const newVal = e.target.value
      setContentMatch({ ...contentMatch, type: newVal })
    }

    function contentMatchChangeHandlerFor(type, name) {
      return (e, newValue) => {
        const copy = { ...contentMatch[type] }
        // newValue is a string for AutoComplete, target.value is set for Select
        copy[name] = typeof newValue === 'string' ? newValue : e.target.value
        const matchCopy = { ...contentMatch }
        matchCopy[type] = copy
        setContentMatch(matchCopy)
      }
    }

    return (
      <Stack direction="column" spacing={2} alignItems="top">
        <Stack direction="row" spacing={2} alignItems="left">
          <FormControl>
            <InputLabel id="contentMatchTypeLabel">Type</InputLabel>
            <Select
              size="small"
              variant="standard"
              labelId="contentMatchTypeLabel"
              label="Type"
              value={contentMatch.type}
              onChange={handleMatchTypeChange}
            >
              <MenuItem value={'none'}>None</MenuItem>
              <MenuItem value={'string'}>String match</MenuItem>
              <MenuItem value={'regex'}>Regex match</MenuItem>
            </Select>
          </FormControl>
          {contentMatch.type === 'string' ? (
            <Autocomplete
              freeSolo={true}
              size="small"
              fullWidth
              options={['Error', 'level=error', 'LimitExceeded']}
              defaultValue=""
              renderInput={(params) => (
                <TextField {...params} label="Match string" />
              )}
              value={contentMatch.string.match}
              onChange={contentMatchChangeHandlerFor('string', 'match')}
            />
          ) : contentMatch.type === 'regex' ? (
            <Autocomplete
              freeSolo={true}
              size="small"
              fullWidth
              options={[
                '[Ee]rror:',
                'Loading information .* for cluster profile',
              ]}
              defaultValue=""
              renderInput={(params) => (
                <TextField {...params} label="Match regex" />
              )}
              value={contentMatch.regex.match}
              onChange={contentMatchChangeHandlerFor('regex', 'match')}
            />
          ) : (
            ''
          )}
        </Stack>
        {['string', 'regex'].includes(contentMatch.type) && (
          <Stack direction="row" spacing={2} alignItems="left">
            <FormControl>
              <InputLabel id="contentMatchLimitLabel">Matches Limit</InputLabel>
              <Select
                autoWidth={true}
                variant="standard"
                size="small"
                sx={{ minWidth: '10em' }}
                labelId="contentMatchLimitLabel"
                label="Matches Limit"
                value={contentMatch[contentMatch.type].limit}
                onChange={contentMatchChangeHandlerFor(
                  contentMatch.type,
                  'limit'
                )}
              >
                {[...Array(12).keys()].map((i) => (
                  <MenuItem key={'contentMatchLimit' + i} value={i + 1}>
                    {i + 1}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            {['Before', 'After'].map((context) => (
              <FormControl key={context.toLowerCase() + 'Lines'}>
                <InputLabel id="contextLinesLabel">
                  {'Context Lines ' + context}
                </InputLabel>
                <Select
                  size="small"
                  variant="standard"
                  sx={{ minWidth: '10em' }}
                  labelId="contextLinesLabel"
                  label={'Context Lines ' + context}
                  value={contentMatch[contentMatch.type][context.toLowerCase()]}
                  onChange={contentMatchChangeHandlerFor(
                    contentMatch.type,
                    context.toLowerCase()
                  )}
                >
                  {[...Array(13).keys()].map((i) => (
                    <MenuItem key={'contextLines' + i} value={i}>
                      {i}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            ))}
          </Stack>
        )}
      </Stack>
    )
  }

  function JAQContentFilterSwitch(props) {
    const { label, display, setDisplay, disabled } = props
    return (
      <FormControl>
        <FormControlLabel
          label={label}
          labelPlacement="end"
          control={
            <Switch
              size="small"
              checked={display}
              onChange={(e) => {
                setDisplay(e.target.checked)
              }}
              disabled={disabled || false}
            />
          }
        />
      </FormControl>
    )
  }
  JAQContentFilterSwitch.propTypes = {
    label: PropTypes.string.isRequired,
    display: PropTypes.bool.isRequired,
    setDisplay: PropTypes.func.isRequired,
    disabled: PropTypes.bool,
  }

  // determine checkbox statuses for the job runs in each row
  function AllSelectingCheckbox() {
    const allSelected = searchJobRunIds.size === selectedJobRunIds.size
    const someSelected =
      selectedJobRunIds.size > 0 &&
      selectedJobRunIds.size < searchJobRunIds.size
    function clickHandler() {
      if (selectedJobRunIds.size > 0) {
        // if any are selected, deselect them
        setSelectedJobRunIds(new Set())
      } else {
        // none selected, select all
        setSelectedJobRunIds(searchJobRunIds)
      }
    }
    return (
      <Tooltip title="Select job runs">
        <Checkbox
          sx={{ padding: 0 }}
          checked={allSelected}
          indeterminate={someSelected}
          disabled={displayOnlySelected}
          label="Select job runs"
          onClick={clickHandler}
        />
      </Tooltip>
    )
  }

  // checkbox overseen by the all-selecting checkbox
  function SelectingCheckbox(props) {
    const { jobRunID } = props
    let jobRunIDSet = new Set([jobRunID])
    let clickHandler = (event) => {
      let selected = event.target.checked
        ? selectedJobRunIds.union(jobRunIDSet)
        : selectedJobRunIds.difference(jobRunIDSet)
      setSelectedJobRunIds(selected)
      if (selected.size === 0) {
        // if we are deselecting the last one, turn off display-only-selected
        // so we have something to select
        setDisplayOnlySelected(false)
      }
    }
    return (
      <Checkbox
        sx={{ padding: 0 }}
        checked={selectedJobRunIds.has(jobRunID)}
        onClick={clickHandler}
      />
    )
  }
  SelectingCheckbox.propTypes = {
    jobRunID: PropTypes.string.isRequired,
  }

  function JAQOpenArtifactsButton(props) {
    const { artifacts } = props

    function handleOpenLinks(event) {
      artifacts.forEach((file) => {
        window.open(file.artifact_url, '_blank')
      })
    }

    if (artifacts.length < 2) return '' // just clutter if there is only one; individual items already linked
    return (
      <Tooltip title="Open all artifacts for this run in browser (NOTE: requires disabling pop-up blockers)">
        <Button size="small" variant="text" onClick={handleOpenLinks}>
          <OpenInNew />
        </Button>
      </Tooltip>
    )
  }
  JAQOpenArtifactsButton.propTypes = {
    artifacts: PropTypes.array.isRequired,
  }

  function JAQResultTable() {
    // snyk is absolutely paranoid about urls coming from state vars. ease its concerns.
    // it would be nice to make this a single "sanitizeURL" function but snyk still doesn't like that.
    function validateURL(url) {
      const parsed = new URL(url)
      return ['https:', 'http:'].includes(parsed.protocol)
    }

    return (
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>
                <AllSelectingCheckbox />
              </TableCell>
              <TableCell>Job Name</TableCell>
              <TableCell>Start Time</TableCell>
              <TableCell>Test Status</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={4}>
                  <CircularProgress />
                </TableCell>
              </TableRow>
            )}
            {filteredRows.map((row) => (
              <Fragment key={row.job_run_id}>
                <TableRow className="cr-artifacts-table-row-jobrun">
                  <TableCell>
                    <SelectingCheckbox jobRunID={row.job_run_id} />
                  </TableCell>
                  <TableCell>
                    <Link
                      // snyk is paranoid about href coming from state vars. use JS instead.
                      onClick={() => window.open(row.url, '_blank')}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {row.job_name}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Tooltip
                      title={
                        new Date(row.start_time).toUTCString() +
                        ' (#' +
                        row.job_run_id +
                        ')'
                      }
                    >
                      <div className="test-name">
                        <Link
                          // snyk is paranoid about href coming from state vars. use JS instead.
                          onClick={() => window.open(row.url, '_blank')}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {relativeTime(new Date(row.start_time), new Date())}
                        </Link>
                      </div>
                    </Tooltip>
                  </TableCell>
                  <TableCell>{row.test_status}</TableCell>
                </TableRow>
                {row.searchResult?.artifacts && (
                  <TableRow className="cr-artifacts-table-row-files">
                    <TableCell>
                      <JAQOpenArtifactsButton
                        artifacts={row.searchResult.artifacts}
                      />
                    </TableCell>
                    <TableCell colSpan={3}>
                      <Stack direction="column" spacing={2}>
                        {row.searchResult.artifacts.map((artifact) => (
                          <Fragment key={artifact.artifact_url}>
                            <Link
                              className="cr-artifacts-truncate"
                              // snyk is paranoid about href coming from state vars. use JS instead.
                              onClick={() =>
                                window.open(artifact.artifact_url, '_blank')
                              }
                              target="_blank"
                              rel="noreferrer noopener"
                            >
                              {artifact.artifact_path}
                            </Link>
                            {artifact.matched_content?.matches &&
                              artifact.matched_content.matches.map(
                                (match, index) => (
                                  <samp
                                    key={index}
                                    className="cr-artifacts-contents"
                                  >
                                    {match.before &&
                                      match.before.map((text, idx) => (
                                        <i key={idx}>
                                          {text}
                                          <br />
                                        </i>
                                      ))}
                                    <b>{match.match}</b>
                                    {match.after &&
                                      match.after.map((text, idx) => (
                                        <i key={idx}>
                                          <br />
                                          {text}
                                        </i>
                                      ))}
                                  </samp>
                                )
                              )}
                          </Fragment>
                        ))}
                      </Stack>
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            ))}
            {filteredRows.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} align="center">
                  No results found
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    )
  }

  function JAQCopyIdsButton() {
    const [copyPopoverEl, setCopyPopoverEl] = React.useState(null)
    const copyPopoverOpen = Boolean(copyPopoverEl)

    function copyIdsToClipboard(event) {
      event.preventDefault()
      let visible = new Set(filteredRows.map((row) => row.job_run_id))
      let selected = selectedJobRunIds.intersection(visible)
      let text = (selected.size > 0 ? selected : visible)
        .keys()
        .toArray()
        .join(' ')
      navigator.clipboard.writeText(text)
      setCopyPopoverEl(event.currentTarget)
      setTimeout(() => setCopyPopoverEl(null), 2000)
    }

    return (
      <Fragment>
        <Tooltip title="Copy selected (or all if none are selected) job run IDs to clipboard">
          <Button size="large" variant="contained" onClick={copyIdsToClipboard}>
            <FileCopy />
            <span>Job Run IDs</span>
          </Button>
        </Tooltip>
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
          IDs copied!
        </Popover>
      </Fragment>
    )
  }

  function JAQOpenJobRunsButton() {
    function handleOpenJobRunLinks(event) {
      let visible = new Set(filteredRows.map((row) => row.job_run_id))
      let selected = selectedJobRunIds.intersection(visible)
      if (!selected.size) selected = visible
      event.preventDefault() //
      filteredRows.forEach((row) => {
        if (selected.has(row.job_run_id)) window.open(row.url, '_blank')
      })
    }
    return (
      <Tooltip title="Open selected (or all if none are selected) job runs in prow (NOTE: requires disabling pop-up blockers)">
        <Button
          size="large"
          variant="contained"
          onClick={handleOpenJobRunLinks}
        >
          <OpenInNew />
          Open job runs
        </Button>
      </Tooltip>
    )
  }

  /*********************************************************************************
   top-level rendering of the entire JobArtifactQuery component
   *********************************************************************************/
  return (
    <Stack
      direction="column"
      spacing={2}
      sx={{ width: '100%', padding: '1em' }}
    >
      <Stack direction="row" spacing={2}>
        <JAQPrefilled
          fileMatch={fileMatch}
          setFileMatch={setFileMatch}
          contentMatch={contentMatch}
          setContentMatch={setContentMatch}
        />
        &nbsp;
      </Stack>
      <div>
        <h3>Artifact File Match</h3>
        <JAQFileMatch />
      </div>
      {fileMatch && (
        <div>
          <h3>Content match</h3>
          <JAQContentMatch />
        </div>
      )}
      <Stack direction="row" spacing={2} alignItems="left">
        <span>Display:</span>
        <JAQContentFilterSwitch
          label="Only selected"
          display={displayOnlySelected}
          setDisplay={setDisplayOnlySelected}
          disabled={selectedJobRunIds.size === 0}
        />
        <JAQContentFilterSwitch
          label="Only runs with artifacts"
          display={displayOnlyArtifacts}
          setDisplay={setDisplayOnlyArtifacts}
          disabled={fileMatch === ''}
        />
        <JAQContentFilterSwitch
          label="Only artifacts with matches"
          display={displayOnlyMatches}
          setDisplay={setDisplayOnlyMatches}
          disabled={contentMatch.type === 'none'}
        />
        {apiCallURL && (
          <Tooltip title="Link to the API call that provides the match data">
            <a href={apiCallURL} target="_blank" rel="noreferrer nofollow">
              <LinkIcon fontSize="small" />
              API URL
            </a>
          </Tooltip>
        )}
      </Stack>
      <JAQResultTable />
      <Stack direction="row" spacing={2}>
        <JAQOpenJobRunsButton />
        <JAQCopyIdsButton />
        <Tooltip title="Return to details report">
          <Button
            size="large"
            variant="contained"
            onClick={handleToggleJAQOpen}
          >
            <Close />
            Close
          </Button>
        </Tooltip>
      </Stack>
    </Stack>
  )
}

JobArtifactQuery.propTypes = {
  searchJobRunIds: PropTypes.object.isRequired,
  jobRunsLookup: PropTypes.instanceOf(Map).isRequired,
  handleToggleJAQOpen: PropTypes.func.isRequired,
}
