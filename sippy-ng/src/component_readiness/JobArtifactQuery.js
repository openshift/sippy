import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Autocomplete,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Divider,
  FormControl,
  FormControlLabel,
  FormHelperText,
  IconButton,
  InputLabel,
  Link,
  MenuItem,
  Paper,
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
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  Add,
  Close,
  Delete,
  Edit as EditIcon,
  ExpandMore,
  FileCopy,
  Link as LinkIcon,
  OpenInNew,
  Preview,
  SavedSearch,
} from '@mui/icons-material'
import { getArtifactQueryAPIUrl } from './CompReadyUtils'
import { relativeTime, safeEncodeURIComponent } from '../helpers'
import { SippyCapabilitiesContext } from '../App'
import LaunderedLink, { openLaunderedLink } from '../components/Laundry'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

const emptyContentMatch = {
  type: 'none',
  none: {},
  string: { match: '', limit: 12, before: 0, after: 0 },
  regex: { match: '', limit: 12, before: 0, after: 0 },
}
const prefilledOptions = new Map([
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
  [
    'LimitExceeded in CCO pod log',
    {
      fileMatch:
        'artifacts/*e2e*/gather-extra/artifacts/pods/openshift-cloud-credential-operator_cloud-credential-operator-*_cloud-credential-operator.log',
      type: 'regex',
      regex: {
        match: 'level=error.*LimitExceeded',
        limit: 12,
        before: 0,
        after: 0,
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
  // Track the currently selected symptom (if any)
  const [selectedSymptom, setSelectedSymptom] = React.useState(null)

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
        if (
          (artifact.matched_content?.line_matches?.matches || []).length > 0
        ) {
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

  // JAQPrefilled is a component to choose from prefilled common queries and symptoms
  function JAQPrefilled() {
    const [prefilled, setPrefilled] = React.useState('')
    const [symptoms, setSymptoms] = React.useState([])

    React.useEffect(() => {
      fetch(process.env.REACT_APP_API_URL + '/api/jobs/symptoms')
        .then((response) => {
          if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`)
          }
          return response.json()
        })
        .then((data) => {
          setSymptoms(
            (data || []).sort((a, b) =>
              (a.summary || '').localeCompare(b.summary || '')
            )
          )
        })
        .catch((error) => {
          console.error('Failed to load symptoms:', error)
        })
    }, [])

    function handlePrefilledChange(e) {
      const selectedValue = e.target.value
      setPrefilled('') // reset selection after each choice populates the dialog

      if (selectedValue === 'reset') {
        // special-cased option to clear all fields
        setFileMatch('')
        setContentMatch(emptyContentMatch)
        setSelectedSymptom(null)
        return
      }

      const params = prefilledOptions.get(selectedValue)
      if (params) {
        // selected a "common query"
        setFileMatch(params.fileMatch)
        setContentMatch({
          ...emptyContentMatch,
          ...params,
        })
        setSelectedSymptom(null)
        return
      }

      // look for a matching symptom
      const symptom = symptoms.find((s) => s.id === selectedValue)
      if (symptom) {
        setFileMatch(symptom.file_pattern || '')

        const newContentMatch = { ...emptyContentMatch }
        if (symptom.matcher_type === 'string') {
          newContentMatch.type = 'string'
          newContentMatch.string = {
            ...emptyContentMatch.string,
            match: symptom.match_string || '',
          }
        } else if (symptom.matcher_type === 'regex') {
          newContentMatch.type = 'regex'
          newContentMatch.regex = {
            ...emptyContentMatch.regex,
            match: symptom.match_string || '',
          }
        } else if (symptom.matcher_type === 'none') {
          newContentMatch.type = 'none'
        }
        setContentMatch(newContentMatch)
        setSelectedSymptom(symptom)
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
              <MenuItem key={-1} value={'reset'}>
                Reset query parameters
              </MenuItem>,
              <MenuItem key="divider-common" disabled>
                ───── Common queries ─────
              </MenuItem>,
              ...prefilledOptions.keys().map((option, index) => (
                <MenuItem key={index} value={option}>
                  {option}
                </MenuItem>
              )),
              ...(symptoms.length === 0
                ? []
                : [
                    <MenuItem key="divider-symptoms" disabled>
                      ───── Symptoms ─────
                    </MenuItem>,
                  ]),
              ...symptoms.map((symptom) => (
                <MenuItem key={symptom.id} value={symptom.id}>
                  {symptom.summary}
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
              sx={{ minWidth: '5em' }}
              variant="standard"
              labelId="contentMatchTypeLabel"
              label="Type"
              value={contentMatch.type}
              onChange={handleMatchTypeChange}
            >
              <MenuItem value={'none'}>None</MenuItem>
              <MenuItem value={'string'}>String</MenuItem>
              <MenuItem value={'regex'}>Regex</MenuItem>
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
            <FormControl className="jaq-form-control">
              <InputLabel id="contentMatchLimitLabel">Matches Limit</InputLabel>
              <Select
                autoWidth={true}
                variant="standard"
                size="small"
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
              <FormControl
                key={context.toLowerCase() + 'Lines'}
                className="jaq-form-control"
              >
                <InputLabel id="contextLinesLabel">
                  {'Lines ' + context}
                </InputLabel>
                <Select
                  size="small"
                  variant="standard"
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
                  <TableCell className="cr-artifacts-jobname">
                    <LaunderedLink
                      address={row.url}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <Tooltip title={row.job_name + ' ' + row.job_run_id}>
                        {row.job_name}/{row.job_run_id}
                      </Tooltip>
                    </LaunderedLink>
                  </TableCell>
                  <TableCell>
                    <Tooltip title={new Date(row.start_time).toUTCString()}>
                      <div className="test-name">
                        <LaunderedLink
                          address={row.url}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {relativeTime(new Date(row.start_time), new Date())}
                        </LaunderedLink>
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
                    <TableCell colSpan={3} className="cr-artifacts-cell">
                      <Stack direction="column" spacing={2}>
                        {row.searchResult.artifacts.map((artifact) => (
                          <Fragment key={artifact.artifact_url}>
                            <LaunderedLink
                              className="cr-artifacts-truncate"
                              address={artifact.artifact_url}
                              target="_blank"
                              rel="noreferrer noopener"
                            >
                              {artifact.artifact_path}
                            </LaunderedLink>
                            {artifact.matched_content?.line_matches
                              ?.matches && (
                              <samp
                                tabIndex="0"
                                key={'samp-' + artifact.artifact_url}
                                className="cr-artifacts-contents"
                              >
                                {artifact.matched_content.line_matches.matches.map(
                                  (match, index) => (
                                    <Fragment
                                      key={index + artifact.artifact_url}
                                    >
                                      {index > 0 && <hr />}
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
                                    </Fragment>
                                  )
                                )}
                              </samp>
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
      event.preventDefault()
      filteredRows.forEach((row) => {
        if (selected.has(row.job_run_id)) {
          // URL comes from trusted backend API, but launder it anyway to satisfy snyk
          openLaunderedLink(row.url)
        }
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

  function JAQSaveAsSymptomSection() {
    const capabilitiesContext = React.useContext(SippyCapabilitiesContext)
    const writeEndpointsEnabled =
      capabilitiesContext.includes('write_endpoints')

    const [expandedSections, setExpandedSections] = React.useState({
      symptom: false,
      labels: true,
      filters: false,
    })
    const [symptomSummary, setSymptomSummary] = React.useState('')
    const [selectedLabelIds, setSelectedLabelIds] = React.useState([])
    const [newLabel, setNewLabel] = React.useState(null)
    const [availableLabels, setAvailableLabels] = React.useState([])
    const [availableSymptoms, setAvailableSymptoms] = React.useState([])
    const [availableReleases, setAvailableReleases] = React.useState([])
    const [selectedReleases, setSelectedReleases] = React.useState([])
    const [selectedReleaseStatuses, setSelectedReleaseStatuses] =
      React.useState([])
    const [selectedProducts, setSelectedProducts] = React.useState([])
    const [saving, setSaving] = React.useState(false)
    const [errorMessage, setErrorMessage] = React.useState('')
    const [successMessage, setSuccessMessage] = React.useState('')

    React.useEffect(() => {
      // Load these once (try again if previous load failed) when opening the symptom section
      if (
        expandedSections.symptom &&
        (availableLabels.length === 0 ||
          availableSymptoms.length === 0 ||
          availableReleases.length === 0)
      ) {
        loadLabels()
        loadSymptoms()
        loadReleases()
      }
    }, [expandedSections.symptom])

    React.useEffect(() => {
      if (!selectedSymptom) return
      setSymptomSummary(selectedSymptom.summary || '')
      setSelectedLabelIds(selectedSymptom.label_ids || [])
      setSelectedReleases(selectedSymptom.filter_releases || [])
      setSelectedReleaseStatuses(selectedSymptom.filter_release_statuses || [])
      setSelectedProducts(selectedSymptom.filter_products || [])
      setNewLabel(null)
      setErrorMessage('')
      setSuccessMessage('')
    }, [selectedSymptom])

    function loadLabels() {
      fetch(process.env.REACT_APP_API_URL + '/api/jobs/labels')
        .then((response) => {
          if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`)
          }
          return response.json()
        })
        .then((data) => {
          setAvailableLabels(
            (data || []).sort((a, b) =>
              (a.label_title || '').localeCompare(b.label_title || '')
            )
          )
        })
        .catch((error) => {
          console.error('Failed to load labels:', error)
          setErrorMessage('Failed to load labels')
        })
    }

    function loadSymptoms() {
      fetch(process.env.REACT_APP_API_URL + '/api/jobs/symptoms')
        .then((response) => {
          if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`)
          }
          return response.json()
        })
        .then((data) => {
          setAvailableSymptoms(
            (data || []).sort((a, b) =>
              (a.summary || '').localeCompare(b.summary || '')
            )
          )
        })
        .catch((error) => {
          console.error('Failed to load symptoms:', error)
          setErrorMessage('Failed to load symptoms')
        })
    }

    function loadReleases() {
      fetch(process.env.REACT_APP_API_URL + '/api/releases')
        .then((response) => {
          if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`)
          }
          return response.json()
        })
        .then((data) => {
          setAvailableReleases(data.releases || [])
        })
        .catch((error) => {
          console.error('Failed to load releases:', error)
          setErrorMessage('Failed to load releases')
        })
    }

    function handleAddNewLabel() {
      setNewLabel({ label_title: '', explanation: '' })
    }

    function handleRemoveNewLabel() {
      setNewLabel(null)
    }

    function handleNewLabelChange(field, value) {
      const updated = { ...newLabel }
      updated[field] = value
      setNewLabel(updated)
    }

    function isSearchValid() {
      if (!fileMatch || fileMatch.length < 3) return false
      if (contentMatch.type === 'string' && !contentMatch.string.match)
        return false
      if (contentMatch.type === 'regex' && !contentMatch.regex.match)
        return false
      return true
    }

    function isSymptomSummaryUniqueForUpdate(summary) {
      if (!summary.trim()) return false
      // When updating an existing symptom, allow keeping the same summary
      if (selectedSymptom && selectedSymptom.summary === summary.trim()) {
        return true
      }
      return isSymptomSummaryUniqueForSave(summary)
    }

    function isSymptomSummaryUniqueForSave(summary) {
      if (!summary.trim()) return false
      // For saving as new, summary must be truly unique (even if it matches selected symptom)
      return !availableSymptoms.some((symptom) => {
        return symptom.summary.toLowerCase() === summary.trim().toLowerCase()
      })
    }

    function isLabelTitleUnique(title) {
      title = title.trim()
      if (!title) return false
      return !availableLabels.some((aLabel) => {
        return aLabel.label_title.toLowerCase() === title.toLowerCase()
      })
    }

    function canSaveNewLabel(newLabel) {
      return newLabel.label_title && isLabelTitleUnique(newLabel.label_title)
    }

    async function handleSaveNewLabel() {
      if (!canSaveNewLabel(newLabel)) {
        setErrorMessage('Label title must be unique and non-empty')
        return
      }

      setSaving(true)
      setErrorMessage('')

      try {
        const response = await fetch(
          process.env.REACT_APP_API_URL + '/api/jobs/labels',
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              ...newLabel,
              label_title: newLabel.label_title.trim(),
            }),
          }
        )
        if (!response.ok) {
          throw new Error(`Failed to create label: ${await response.text()}`)
        }
        const created = await response.json()

        setSelectedLabelIds([...selectedLabelIds, created.id])
        setNewLabel(null)
        loadLabels()
        setSuccessMessage(
          `Label "${created.label_title}" created successfully!`
        )
      } catch (error) {
        console.error('Error saving label:', error)
        setErrorMessage(error.message)
      } finally {
        setSaving(false)
      }
    }

    function toggleSection(section) {
      setExpandedSections({
        ...expandedSections,
        [section]: !expandedSections[section],
      })
    }

    function hasSymptomChanged() {
      if (!selectedSymptom) return false

      const matcherType =
        contentMatch.type === 'none' ? 'none' : contentMatch.type
      const currentMatchString =
        matcherType === 'string'
          ? contentMatch.string.match
          : matcherType === 'regex'
          ? contentMatch.regex.match
          : ''

      const allLabelIds = [...selectedLabelIds]

      // make copies so we can sort for comparison without changing state
      function arraysSortEqual(arr1, arr2) {
        return (
          JSON.stringify([...(arr1 || [])].sort()) ===
          JSON.stringify([...(arr2 || [])].sort())
        )
      }

      return (
        symptomSummary !== selectedSymptom.summary ||
        fileMatch !== selectedSymptom.file_pattern ||
        matcherType !== selectedSymptom.matcher_type ||
        currentMatchString !== (selectedSymptom.match_string || '') ||
        !arraysSortEqual(allLabelIds, selectedSymptom.label_ids) ||
        !arraysSortEqual(selectedReleases, selectedSymptom.filter_releases) ||
        !arraysSortEqual(
          selectedReleaseStatuses,
          selectedSymptom.filter_release_statuses
        ) ||
        !arraysSortEqual(selectedProducts, selectedSymptom.filter_products)
      )
    }

    async function handleSymptomUpdate() {
      setErrorMessage('')
      setSuccessMessage('')
      setSaving(true)

      try {
        const matcherType =
          contentMatch.type === 'none' ? 'none' : contentMatch.type
        const symptom = {
          id: selectedSymptom.id,
          summary: symptomSummary,
          matcher_type: matcherType,
          file_pattern: fileMatch,
          match_string:
            matcherType === 'string'
              ? contentMatch.string.match
              : matcherType === 'regex'
              ? contentMatch.regex.match
              : '',
          label_ids: selectedLabelIds,
          filter_releases:
            selectedReleases.length > 0 ? selectedReleases : null,
          filter_release_statuses:
            selectedReleaseStatuses.length > 0 ? selectedReleaseStatuses : null,
          filter_products:
            selectedProducts.length > 0 ? selectedProducts : null,
        }

        const response = await fetch(selectedSymptom.links['self'], {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(symptom),
        })

        if (!response.ok) {
          const errorText = await response.text()
          throw new Error(`Failed to update symptom: ${errorText}`)
        }

        const updatedSymptom = await response.json()
        setSuccessMessage('Symptom updated successfully!')
        setSelectedSymptom(updatedSymptom)
        setNewLabel(null)
        loadLabels()
      } catch (error) {
        console.error('Error updating symptom:', error)
        setErrorMessage(error.message)
      } finally {
        setSaving(false)
      }
    }

    async function handleSymptomSave() {
      setErrorMessage('')
      setSuccessMessage('')

      if (!isSymptomSummaryUniqueForSave(symptomSummary)) {
        setErrorMessage('Symptom name must be unique')
        return
      }

      setSaving(true)

      try {
        const matcherType =
          contentMatch.type === 'none' ? 'none' : contentMatch.type
        const symptom = {
          summary: symptomSummary,
          matcher_type: matcherType,
          file_pattern: fileMatch,
          match_string:
            matcherType === 'string'
              ? contentMatch.string.match
              : matcherType === 'regex'
              ? contentMatch.regex.match
              : '',
          label_ids: selectedLabelIds,
          filter_releases:
            selectedReleases.length > 0 ? selectedReleases : null,
          filter_release_statuses:
            selectedReleaseStatuses.length > 0 ? selectedReleaseStatuses : null,
          filter_products:
            selectedProducts.length > 0 ? selectedProducts : null,
        }

        const response = await fetch(
          process.env.REACT_APP_API_URL + '/api/jobs/symptoms',
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(symptom),
          }
        )

        if (!response.ok) {
          const errorText = await response.text()
          throw new Error(`Failed to create symptom: ${errorText}`)
        }

        setNewLabel(null)
        setSelectedSymptom(await response.json())
        setSuccessMessage('Symptom created successfully!')
        loadLabels()
        loadSymptoms()
      } catch (error) {
        console.error('Error saving symptom:', error)
        setErrorMessage(error.message)
      } finally {
        setSaving(false)
      }
    }

    // Don't show the section if server write endpoints are not enabled; must be authorized to write
    if (!writeEndpointsEnabled) {
      return null
    }

    return (
      <Accordion
        expanded={expandedSections.symptom}
        onChange={() => toggleSection('symptom')}
      >
        <AccordionSummary expandIcon={<ExpandMore />}>
          <SavedSearch sx={{ mr: 1 }} />
          <Typography>Save as Symptom</Typography>
        </AccordionSummary>
        <AccordionDetails>
          <Stack spacing={2}>
            {errorMessage && <Alert severity="error">{errorMessage}</Alert>}
            {successMessage && (
              <Alert severity="success">{successMessage}</Alert>
            )}

            <Typography variant="caption" color="text.secondary">
              Save this artifact query as a reusable symptom definition that can
              automatically apply labels to matching job runs.
            </Typography>

            {!isSearchValid() && (
              <Alert severity="warning">
                Complete the file pattern and content match criteria above
                before saving as a symptom.
              </Alert>
            )}

            {selectedSymptom && (
              <Alert severity="info">
                Editing symptom: <strong>{selectedSymptom.summary}</strong>
                {hasSymptomChanged() && ' (modified)'}
              </Alert>
            )}

            <TextField
              label="Symptom Summary"
              fullWidth
              required
              value={symptomSummary}
              onChange={(e) => setSymptomSummary(e.target.value)}
              error={
                symptomSummary.trim() &&
                !isSymptomSummaryUniqueForUpdate(symptomSummary)
              }
              helperText={
                symptomSummary.trim() &&
                !isSymptomSummaryUniqueForUpdate(symptomSummary)
                  ? 'Symptom summary must be unique'
                  : 'Brief descriptive summary (must be unique)'
              }
              size="small"
            />

            <Accordion
              expanded={expandedSections.labels}
              onChange={() => toggleSection('labels')}
            >
              <AccordionSummary expandIcon={<ExpandMore />}>
                <Typography>
                  Labels to Apply (Optional)
                  {selectedLabelIds.length > 0 && (
                    <Chip
                      label={selectedLabelIds.length}
                      size="small"
                      sx={{ ml: 1 }}
                    />
                  )}
                </Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Stack spacing={2}>
                  <Autocomplete
                    multiple
                    options={availableLabels}
                    getOptionLabel={(option) => option.label_title || option.id}
                    value={availableLabels.filter((aLabel) =>
                      selectedLabelIds.includes(aLabel.id)
                    )}
                    onChange={(event, newValues) => {
                      setSelectedLabelIds(newValues.map((v) => v.id))
                    }}
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        label="Select Existing Labels"
                        helperText="Choose labels to apply when this symptom matches"
                      />
                    )}
                    renderTags={(value, getTagProps) =>
                      value.map((option, index) => (
                        <Chip
                          label={option.label_title || option.id}
                          {...getTagProps({ index })}
                          key={option.id}
                        />
                      ))
                    }
                  />

                  <Divider>
                    <Typography variant="caption">Or create new</Typography>
                  </Divider>

                  {newLabel ? (
                    <Stack spacing={1}>
                      <Stack
                        direction="row"
                        spacing={1}
                        alignItems="flex-start"
                      >
                        <Stack spacing={1} sx={{ flex: 1 }}>
                          <TextField
                            label="New Label Title"
                            size="small"
                            fullWidth
                            required
                            value={newLabel.label_title}
                            onChange={(e) =>
                              handleNewLabelChange(
                                'label_title',
                                e.target.value
                              )
                            }
                            error={
                              newLabel.label_title &&
                              !isLabelTitleUnique(newLabel.label_title)
                            }
                            helperText={
                              newLabel.label_title &&
                              !isLabelTitleUnique(newLabel.label_title)
                                ? 'Label title must be unique'
                                : ''
                            }
                          />
                          <MarkdownEditor
                            value={newLabel.explanation}
                            onChange={(e) =>
                              handleNewLabelChange(
                                'explanation',
                                e.target.value
                              )
                            }
                            label="Explanation (optional, markdown supported)"
                            helperText="Use markdown formatting for rich text"
                          />
                        </Stack>
                        <IconButton
                          onClick={handleRemoveNewLabel}
                          size="small"
                          color="error"
                        >
                          <Delete />
                        </IconButton>
                      </Stack>
                      <Button
                        onClick={handleSaveNewLabel}
                        variant="contained"
                        size="small"
                        disabled={!canSaveNewLabel(newLabel) || saving}
                      >
                        Create Label
                      </Button>
                    </Stack>
                  ) : (
                    <Button
                      startIcon={<Add />}
                      onClick={handleAddNewLabel}
                      variant="outlined"
                      size="small"
                    >
                      Add New Label
                    </Button>
                  )}
                </Stack>
              </AccordionDetails>
            </Accordion>

            <Accordion
              expanded={expandedSections.filters}
              onChange={() => toggleSection('filters')}
            >
              <AccordionSummary expandIcon={<ExpandMore />}>
                <Typography>
                  Applicability Filters (Optional){' '}
                  {(selectedReleases.length > 0 ||
                    selectedReleaseStatuses.length > 0 ||
                    selectedProducts.length > 0) && (
                    <Chip
                      label={
                        selectedReleases.length +
                        selectedReleaseStatuses.length +
                        selectedProducts.length
                      }
                      size="small"
                      sx={{ ml: 1 }}
                    />
                  )}
                </Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Stack spacing={2}>
                  <Typography variant="caption" color="text.secondary">
                    Apply labels only on job runs included by filters.
                  </Typography>

                  <FormControl fullWidth>
                    <InputLabel id="releases-label">Releases</InputLabel>
                    <Select
                      labelId="releases-label"
                      label="Releases"
                      multiple
                      value={selectedReleases}
                      onChange={(e) => setSelectedReleases(e.target.value)}
                      renderValue={(selected) => (
                        <Stack direction="row" spacing={0.5} flexWrap="wrap">
                          {selected.map((value) => (
                            <Chip key={value} label={value} size="small" />
                          ))}
                        </Stack>
                      )}
                    >
                      {availableReleases.map((release) => (
                        <MenuItem key={release} value={release}>
                          <Checkbox
                            checked={selectedReleases.indexOf(release) > -1}
                          />
                          {release}
                        </MenuItem>
                      ))}
                    </Select>
                    <FormHelperText>
                      Filter to specific releases or leave empty for all
                    </FormHelperText>
                  </FormControl>

                  <FormControl fullWidth>
                    <InputLabel id="release-statuses-label">
                      Release Statuses
                    </InputLabel>
                    <Select
                      labelId="release-statuses-label"
                      label="Release Statuses"
                      multiple
                      value={selectedReleaseStatuses}
                      onChange={(e) =>
                        setSelectedReleaseStatuses(e.target.value)
                      }
                      renderValue={(selected) => (
                        <Stack direction="row" spacing={0.5} flexWrap="wrap">
                          {selected.map((value) => (
                            <Chip key={value} label={value} size="small" />
                          ))}
                        </Stack>
                      )}
                    >
                      {[
                        'Development',
                        'Full Support',
                        'Maintenance Support',
                        'Extended Support',
                        'End of Life',
                      ].map((status) => (
                        <MenuItem key={status} value={status}>
                          <Checkbox
                            checked={
                              selectedReleaseStatuses.indexOf(status) > -1
                            }
                          />
                          {status}
                        </MenuItem>
                      ))}
                    </Select>
                    <FormHelperText>
                      Filter to specific release statuses or leave empty for all
                    </FormHelperText>
                  </FormControl>

                  <FormControl fullWidth>
                    <InputLabel id="products-label">Products</InputLabel>
                    <Select
                      labelId="products-label"
                      label="Products"
                      multiple
                      value={selectedProducts}
                      onChange={(e) => setSelectedProducts(e.target.value)}
                      renderValue={(selected) => (
                        <Stack direction="row" spacing={0.5} flexWrap="wrap">
                          {selected.map((value) => (
                            <Chip key={value} label={value} size="small" />
                          ))}
                        </Stack>
                      )}
                    >
                      {['OCP', 'OKD', 'HCM'].map((product) => (
                        <MenuItem key={product} value={product}>
                          <Checkbox
                            checked={selectedProducts.indexOf(product) > -1}
                          />
                          {product}
                        </MenuItem>
                      ))}
                    </Select>
                    <FormHelperText>
                      Filter to specific products or leave empty for all
                    </FormHelperText>
                  </FormControl>
                </Stack>
              </AccordionDetails>
            </Accordion>

            <Stack direction="row" spacing={2} justifyContent="flex-end">
              {selectedSymptom && hasSymptomChanged() && (
                <Button
                  onClick={handleSymptomUpdate}
                  variant="contained"
                  color="primary"
                  disabled={
                    saving ||
                    !symptomSummary.trim() ||
                    !isSearchValid() ||
                    !isSymptomSummaryUniqueForUpdate(symptomSummary)
                  }
                  startIcon={
                    saving ? <CircularProgress size={20} /> : <SavedSearch />
                  }
                >
                  {saving ? 'Updating...' : 'Update Symptom'}
                </Button>
              )}
              <Button
                onClick={handleSymptomSave}
                variant={
                  selectedSymptom && hasSymptomChanged()
                    ? 'outlined'
                    : 'contained'
                }
                disabled={
                  saving ||
                  !symptomSummary.trim() ||
                  !isSearchValid() ||
                  !isSymptomSummaryUniqueForSave(symptomSummary)
                }
                startIcon={
                  saving ? <CircularProgress size={20} /> : <SavedSearch />
                }
              >
                {saving ? 'Saving...' : 'Save as New Symptom'}
              </Button>
            </Stack>
          </Stack>
        </AccordionDetails>
      </Accordion>
    )
  }

  function MarkdownEditor(props) {
    const { value, onChange, label, helperText } = props
    const [viewMode, setViewMode] = React.useState('edit')

    return (
      <Stack spacing={1}>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="caption" color="text.secondary">
            {label}
          </Typography>
          <ToggleButtonGroup
            value={viewMode}
            exclusive
            onChange={(e, newMode) => {
              if (newMode !== null) setViewMode(newMode)
            }}
            size="small"
          >
            <ToggleButton value="edit">
              <EditIcon fontSize="small" />
              <Typography variant="caption" sx={{ ml: 0.5 }}>
                Edit
              </Typography>
            </ToggleButton>
            <ToggleButton value="preview">
              <Preview fontSize="small" />
              <Typography variant="caption" sx={{ ml: 0.5 }}>
                Preview
              </Typography>
            </ToggleButton>
          </ToggleButtonGroup>
        </Stack>

        {viewMode === 'edit' ? (
          <TextField
            fullWidth
            multiline
            rows={4}
            value={value}
            onChange={onChange}
            helperText={helperText}
            placeholder="Enter markdown text..."
            size="small"
          />
        ) : (
          <Paper
            variant="outlined"
            sx={{
              p: 2,
              minHeight: '120px',
              backgroundColor: (theme) =>
                theme.palette.mode === 'dark'
                  ? 'rgba(255, 255, 255, 0.05)'
                  : 'grey.50',
            }}
          >
            {value ? (
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{value}</ReactMarkdown>
            ) : (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ fontStyle: 'italic' }}
              >
                No content to preview
              </Typography>
            )}
          </Paper>
        )}
      </Stack>
    )
  }
  MarkdownEditor.propTypes = {
    value: PropTypes.string.isRequired,
    onChange: PropTypes.func.isRequired,
    label: PropTypes.string,
    helperText: PropTypes.string,
  }

  /*********************************************************************************
   Main component render
   *********************************************************************************/
  return (
    <Stack
      direction="column"
      spacing={2}
      sx={{ width: '100%', padding: '1em' }}
    >
      <Stack direction="row" spacing={2}>
        <JAQPrefilled />
        &nbsp;
      </Stack>
      <Stack direction="row" spacing={2}>
        <div className="jaq-control-box">
          <h3>Artifact File Match</h3>
          <JAQFileMatch />
        </div>
        {fileMatch && (
          <div className="jaq-control-box">
            <h3>Content match</h3>
            <JAQContentMatch />
          </div>
        )}
      </Stack>
      <JAQSaveAsSymptomSection />
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
          <LaunderedLink
            address={apiCallURL}
            target="_blank"
            rel="noopener noreferrer"
          >
            <Tooltip title="Link to the API call that provides the match data">
              <LinkIcon fontSize="small" />
              API URL
            </Tooltip>
          </LaunderedLink>
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
