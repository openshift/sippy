import * as lodash from 'lodash'
import {
  ArrayParam,
  encodeQueryParams,
  StringParam,
  useQueryParam,
} from 'use-query-params'
import { Button, ButtonGroup, MenuItem, Select, TextField } from '@mui/material'
import { CircularProgress } from '@mui/material'
import { stringify } from 'query-string'
import { useHistory } from 'react-router-dom'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect, useState } from 'react'
import TimelineChart from '../components/TimelineChart'

export default function ProwJobRun(props) {
  const history = useHistory()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [eventIntervals, setEventIntervals] = React.useState([])
  const [filteredIntervals, setFilteredIntervals] = React.useState([])

  // categories is the set of selected categories to display. It is controlled by a combination
  // of default props, the categories query param, and the buttons the user can modify with.
  const [selectedSources = props.selectedSources, setSelectedSources] =
    useQueryParam('selectedSources', ArrayParam)

  const [allIntervalFiles, setAllIntervalFiles] = useState([])
  const [allSources, setAllSources] = useState([])
  const [intervalFile = props.intervalFile, setIntervalFile] = useState(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('intervalFile')) {
      console.log(
        'returning intervalFile from URL search params: ' +
          params.get('intervalFile')
      )
      return params.get('intervalFile')
    }
    return ''
  })

  const [filterText, setFilterText] = useState(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('filter')) {
      return params.get('filter')
    }
    return ''
  })

  const fetchData = () => {
    let queryString = ''
    console.log(
      'fetching new data: jobRun=' +
        props.jobRunID +
        ', jobName=' +
        props.jobName +
        ', pullNumber=' +
        props.pullNumber +
        ', repoInfo=' +
        props.repoInfo +
        ', intervalFile=' +
        intervalFile
    )

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/jobs/runs/intervals?prow_job_run_id=' +
        props.jobRunID +
        (props.jobName ? '&job_name=' + props.jobName : '') +
        (props.repoInfo ? '&repo_info=' + props.repoInfo : '') +
        (props.pullNumber ? '&pull_number=' + props.pullNumber : '') +
        (intervalFile ? '&file=' + intervalFile : '') +
        queryString
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        if (json != null) {
          // Process and filter our intervals
          let tmpIntervals = json.items
          mutateIntervals(tmpIntervals)
          setEventIntervals(tmpIntervals)

          let intervalFilesAvailable = json.intervalFilesAvailable
          intervalFilesAvailable.sort()
          setAllIntervalFiles(intervalFilesAvailable)
          let allSources = []
          lodash.forEach(tmpIntervals, function (eventInterval) {
            if (!allSources.includes(eventInterval.source)) {
              allSources.push(eventInterval.source)
            }
          })
          console.log('allSources = ' + allSources)
          setAllSources(allSources)
          // This is a little tricky, we do a query first without specifying a filename, as we don't know what
          // files are available. The server makes a best guess and returns the intervals for that file, as well as
          // a list of all available file names. In the UI if we don't yet have one, populate the select with the value
          // we received.
          if (intervalFile == '') {
            console.log(
              'setting interval file to first intervals filename: ' +
                json.items[0].filename
            )
            // TODO: Causes a duplicate API request when we set this. Look into useRef and only calling
            // fetchData in useEffect if we've made it through an initial page load?
            setIntervalFile(json.items[0].filename)
          }
        } else {
          setEventIntervals([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve intervals for ' +
            'jobRunID=' +
            props.jobRunID +
            ' jobName=' +
            props.jobName +
            ' pullNumber=' +
            props.pullNumber +
            ' repoInfo=' +
            ', ' +
            error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [intervalFile])

  useEffect(() => {
    updateFiltering()
  }, [selectedSources, history, eventIntervals])

  useEffect(() => {
    // Delayed processing of the filter text input to allow the user to finish typing before
    // we update our filtering:
    const timer = setTimeout(() => {
      console.log('Filter text updated:', filterText)
      updateFiltering()
    }, 500)

    return () => clearTimeout(timer)
  }, [filterText])

  function updateFiltering() {
    console.log('updating filtering')

    let queryString = encodeQueryParams(
      {
        selectedSources: ArrayParam,
        intervalFile: StringParam,
        filter: StringParam,
      },
      { selectedSources, intervalFile, filterText }
    )

    history.replace({
      search: stringify(queryString),
    })

    let filteredIntervals = filterIntervals(
      eventIntervals,
      selectedSources,
      filterText
    )
    setFilteredIntervals(filteredIntervals)
  }

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return (
      <Fragment>
        <p>
          Loading intervals for job run: jobRunID={props.jobRunID}, jobName=
          {props.jobName}, pullNumber={props.pullNumber}, repoInfo=
          {props.repoInfo}
        </p>
        <CircularProgress />
      </Fragment>
    )
  }

  let chartData = groupIntervals(selectedSources, filteredIntervals)

  function handleCategoryClick(buttonValue) {
    console.log('got category button click: ' + buttonValue)
    const newSources = [...selectedSources]
    const selectedIndex = selectedSources.indexOf(buttonValue)

    if (selectedIndex === -1) {
      console.log(buttonValue + ' is now selected')
      newSources.push(buttonValue)
    } else {
      console.log(buttonValue + ' is no longer selected')
      newSources.splice(selectedIndex, 1)
    }

    console.log('new selectedSources: ' + newSources)
    setSelectedSources(newSources)
  }

  /*
  function handleIntervalFileChange(buttonValue) {
    console.log('got interval file button click: ' + buttonValue)
    const newSelectedIntervalFile = [...intervalFiles]
    const selectedIndex = intervalFiles.indexOf(buttonValue)

    if (selectedIndex === -1) {
      console.log(buttonValue + ' is now selected')
      newSelectedIntervalFiles.push(buttonValue)
    } else {
      console.log(buttonValue + ' is no longer selected')
      newSelectedIntervalFiles.splice(selectedIndex, 1)
    }

    console.log('new selected interval files: ' + newSelectedIntervalFiles)
    setIntervalFiles(newSelectedIntervalFiles)
  }

   */
  const handleIntervalFileChange = (event) => {
    console.log('new interval file selected: ' + event.target.value)
    setIntervalFile(event.target.value)
  }

  const handleFilterChange = (event) => {
    setFilterText(event.target.value)
  }

  // handleSegmentClicked is called whenever an individual interval in the chart is clicked.
  // Used to display details on the interval and locator in a way that a user can copy if needed.
  function handleSegmentClicked(segment) {
    // Copy label to clipboard
    navigator.clipboard.writeText(segment.labelVal)
  }

  function segmentTooltipFunc(d) {
    return (
      '<span style="max-inline-size: min-content; display: inline-block;">' +
      '<strong>' +
      d.labelVal +
      '</strong><br/>' +
      '<strong>From: </strong>' +
      new Date(d.timeRange[0]).toUTCString() +
      '<br>' +
      '<strong>To: </strong>' +
      new Date(d.timeRange[1]).toUTCString() +
      '</span>'
    )
  }

  return (
    <Fragment>
      <p>
        Loaded {eventIntervals.length} intervals from GCS, filtered down to{' '}
        {filteredIntervals.length}.
      </p>
      <div>
        Categories:
        <ButtonGroup size="small" aria-label="Categories">
          {allSources.map((source) => (
            <Button
              key={source}
              onClick={() => handleCategoryClick(source)}
              variant={
                selectedSources.includes(source) ? 'contained' : 'outlined'
              }
            >
              {source}
            </Button>
          ))}
        </ButtonGroup>
      </div>
      <div>
        Files:
        <Select
          labelId="interval-file-label"
          id="interval-file"
          value={intervalFile}
          label="Interval File"
          onChange={handleIntervalFileChange}
        >
          {allIntervalFiles.map((iFile) => (
            <MenuItem key={iFile} value={iFile}>
              {iFile}
            </MenuItem>
          ))}
        </Select>
        <TextField
          id="filter"
          label="Regex Filter"
          variant="outlined"
          onChange={handleFilterChange}
          defaultValue={filterText}
        />
      </div>
      <div>
        <TimelineChart
          data={chartData}
          eventIntervals={filteredIntervals}
          segmentClickedFunc={handleSegmentClicked}
          segmentTooltipContentFunc={segmentTooltipFunc}
        />
      </div>
    </Fragment>
  )
}

ProwJobRun.defaultProps = {}

ProwJobRun.defaultProps = {
  // default list of pre-selected sources:
  selectedSources: [
    'OperatorAvailable',
    'OperatorProgressing',
    'OperatorDegraded',
    'KubeletLog',
    'EtcdLog',
    'EtcdLeadership',
    'Alert',
    'Disruption',
    'E2ETest',
    'APIServerGracefulShutdown',
    'KubeEvent',
    'NodeState',
  ],
  intervalFile: '',
}

ProwJobRun.propTypes = {
  selectedSources: PropTypes.array,
  intervalFile: PropTypes.string,
}

ProwJobRun.propTypes = {
  jobRunID: PropTypes.string.isRequired,
  jobName: PropTypes.string,
  repoInfo: PropTypes.string,
  pullNumber: PropTypes.string,
  filterModel: PropTypes.object,
}

function filterIntervals(eventIntervals, selectedSources, filterText) {
  let re = null
  if (filterText) {
    re = new RegExp(filterText)
  }

  // TODO: Filter on display = true?

  return _.filter(eventIntervals, function (eventInterval) {
    let shouldInclude = false
    // Go ahead and filter out uncategorized events
    Object.keys(eventInterval.categories).forEach(function (cat) {
      //      if (eventInterval.categories[cat] && selectedSources.includes(cat)) {
      if (re) {
        if (re.test(eventInterval.message) || re.test(eventInterval.locator)) {
          shouldInclude = true
        }
      } else {
        shouldInclude = true
      }
      //     }
    })
    return shouldInclude
  })
}

function mutateIntervals(eventIntervals) {
  // Structure the locator data and then categorize the event
  lodash.forEach(eventIntervals, function (eventInterval) {
    // TODO Wasn't clear if an event is only supposed to be in one category or if it can show up in multiple, with the existing implementation
    // it can show up more than once if it passes more than one of the category checks. If it is meant to only be one category this
    // could be something simpler like eventInterval.category = "operator-degraded" instead.
    // Not hypthetical, found events that passed isPodLogs also passed isPods.

    // Hack until https://issues.redhat.com/browse/TRT-1653 is fixed.
    if (eventInterval.locator.keys === null) {
      eventInterval.locator.keys = {}
    }

    // Hack to split the OperatorSource intervals into "fake" sources of Progressing
    // Available and Degraded:
    if (eventInterval.source === 'OperatorState') {
      if (eventInterval.message.annotations.condition === 'Available') {
        eventInterval.source = 'OperatorAvailable'
      } else if (
        eventInterval.message.annotations.condition === 'Progressing'
      ) {
        eventInterval.source = 'OperatorProgressing'
      } else if (eventInterval.message.annotations.condition === 'Degraded') {
        eventInterval.source = 'OperatorDegraded'
      }
    }

    // Categorizing the events once on page load will save time on filtering later
    eventInterval.categories = {}
    eventInterval.categories.operator_unavailable =
      isOperatorAvailable(eventInterval)
    eventInterval.categories.operator_progressing =
      isOperatorProgressing(eventInterval)
    eventInterval.categories.operator_degraded =
      isOperatorDegraded(eventInterval)
    eventInterval.categories.pods = isPod(eventInterval)
    eventInterval.categories.pod_logs = isPodLog(eventInterval)
    eventInterval.categories.system_journal = isSystemJournalLog(eventInterval)
    eventInterval.categories.interesting_events =
      isInterestingOrPathological(eventInterval)
    eventInterval.categories.alerts = isAlert(eventInterval)
    eventInterval.categories.node_state = isNodeState(eventInterval)
    eventInterval.categories.e2e_test_failed = isE2EFailed(eventInterval)
    eventInterval.categories.e2e_test_flaked = isE2EFlaked(eventInterval)
    eventInterval.categories.e2e_test_passed = isE2EPassed(eventInterval)
    eventInterval.categories.disruption = isEndpointConnectivity(eventInterval)
    eventInterval.categories.apiserver_shutdown =
      isGracefulShutdownActivity(eventInterval)
    eventInterval.categories.etcd_leaders =
      isEtcdLeadershipAndNotEmpty(eventInterval)
    eventInterval.categories.cloud_metrics = isCloudMetrics(eventInterval)
    eventInterval.categories.uncategorized = !_.some(eventInterval.categories) // will save time later during filtering and re-rendering since we don't render any uncategorized events

    // Calculate the string representation of the message (tooltip) and locator once on load:
    eventInterval.displayMessage = defaultToolTip(eventInterval)
    eventInterval.displayLocator = buildLocatorDisplayString(
      eventInterval.locator
    )
  })
}

function groupIntervals(selectedSources, filteredIntervals) {
  let timelineGroups = []
  console.log('grouping intervals for selected sources: ' + selectedSources)

  selectedSources.forEach((source) => {
    timelineGroups.push({ group: source, data: [] })
    createTimelineData(
      source,
      timelineGroups[timelineGroups.length - 1].data,
      filteredIntervals,
      source
    )
    console.log(
      'pushed ' +
        timelineGroups[timelineGroups.length - 1].data.length +
        ' intervals for source: ' +
        source
    )
  })

  /*
  timelineGroups.push({ group: 'operator-unavailable', data: [] })
  createTimelineData(
    'OperatorUnavailable',
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'operator_unavailable'
  )

  timelineGroups.push({ group: 'operator-degraded', data: [] })
  createTimelineData(
    'OperatorDegraded',
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'operator_degraded'
  )

  timelineGroups.push({ group: 'operator-progressing', data: [] })
  createTimelineData(
    'OperatorProgressing',
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'operator_progressing'
  )

  timelineGroups.push({ group: 'pods', data: [] })
  createTimelineData(
    podStateValue,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'pods'
  )
  timelineGroups[timelineGroups.length - 1].data.sort(function (e1, e2) {
    // I think I really want ordering by time in each of a few categories
    return e1.label < e2.label ? -1 : e1.label > e2.label
  })

  timelineGroups.push({ group: 'pod-logs', data: [] })
  createTimelineData(
    podLogs,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'pod_logs'
  )

  timelineGroups.push({ group: 'system-journal', data: [] })
  createTimelineData(
    journalLogs,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'system_journal'
  )

  timelineGroups.push({ group: 'alerts', data: [] })
  createTimelineData(
    alertSeverity,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'alerts'
  )
  // leaving this for posterity so future me (or someone else) can try it, but I think ordering by name makes the
  // patterns shown by timing hide and timing appears more relevant to my eyes.
  // sort alerts alphabetically for display purposes, but keep the json itself ordered by time.
  // timelineGroups[timelineGroups.length - 1].data.sort(function (e1 ,e2){
  //     if (e1.label.includes("alert") && e2.label.includes("alert")) {
  //         return e1.label < e2.label ? -1 : e1.label > e2.label;
  //     }
  //     return 0
  // })

  timelineGroups.push({ group: 'node-state', data: [] })
  createTimelineData(
    nodeStateValue,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'node_state'
  )
  // Sort the node-state intervals so rows are grouped by node
  timelineGroups[timelineGroups.length - 1].data.sort(function (e1, e2) {
    return e1.label < e2.label ? -1 : e1.label > e2.label
  })

  timelineGroups.push({ group: 'etcd-leaders', data: [] })
  createTimelineData(
    etcdLeadershipLogsValue,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'etcd_leaders'
  )

  timelineGroups.push({ group: 'cloud-metrics', data: [] })
  createTimelineData(
    cloudMetricsValue,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'cloud_metrics'
  )

  timelineGroups.push({ group: 'disruption', data: [] })
  createTimelineData(
    disruptionValue,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'disruption'
  )

  timelineGroups.push({ group: 'apiserver-shutdown', data: [] })
  createTimelineData(
    apiserverShutdownValue,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'apiserver_shutdown'
  )

  timelineGroups.push({ group: 'e2e-test-failed', data: [] })
  createTimelineData(
    'Failed',
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'e2e_test_failed'
  )

  timelineGroups.push({ group: 'e2e-test-flaked', data: [] })
  createTimelineData(
    'Flaked',
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'e2e_test_flaked'
  )

  timelineGroups.push({ group: 'e2e-test-passed', data: [] })
  createTimelineData(
    'Passed',
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'e2e_test_passed'
  )

  timelineGroups.push({ group: 'interesting-events', data: [] })
  createTimelineData(
    interestingEvents,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'interesting_events'
  )
       */
  return timelineGroups
}

function isOperatorAvailable(eventInterval) {
  return (
    eventInterval.locator.type === 'ClusterOperator' &&
    eventInterval.message.annotations['condition'] === 'Available' &&
    eventInterval.message.annotations['status'] === 'False'
  )
}

function isOperatorDegraded(eventInterval) {
  return (
    eventInterval.locator.type === 'ClusterOperator' &&
    eventInterval.message.annotations['condition'] === 'Degraded' &&
    eventInterval.message.annotations['status'] === 'True'
  )
}

function isOperatorProgressing(eventInterval) {
  return (
    eventInterval.locator.type === 'ClusterOperator' &&
    eventInterval.message.annotations['condition'] === 'Progressing' &&
    eventInterval.message.annotations['status'] === 'True'
  )
}

// When an interval in the openshift-etcd namespace had a reason of LeaderFound, LeaderLost,
// LeaderElected, or LeaderMissing, source was set to 'EtcdLeadership'.
function isEtcdLeadership(eventInterval) {
  return eventInterval.source === 'EtcdLeadership'
}

function isPodLog(eventInterval) {
  if (eventInterval.source === 'PodLog') {
    return true
  }
  return eventInterval.source === 'EtcdLog'
}

function isSystemJournalLog(eventInterval) {
  if (eventInterval.source === 'OVSVswitchdLog') {
    console.log('found one')
    return true
  }
  // TODO: may want to add more here in future
  return false
}

function isInterestingOrPathological(eventInterval) {
  return (
    eventInterval.source === 'KubeEvent' &&
    eventInterval.message.annotations['pathological'] === 'true'
  )
}

function isPod(eventInterval) {
  return eventInterval.source === 'PodState'
}

function isPodLifecycle(eventInterval) {
  return (
    eventInterval.source === 'PodState' &&
    (eventInterval.message.reason === 'Created' ||
      eventInterval.message.reason === 'Scheduled' ||
      eventInterval.message.reason === 'GracefulDelete')
  )
}

function isContainerLifecycle(eventInterval) {
  return (
    eventInterval.source === 'PodState' &&
    (eventInterval.message.reason === 'ContainerExit' ||
      eventInterval.message.reason === 'ContainerStart' ||
      eventInterval.message.reason === 'ContainerWait')
  )
}

function isContainerReadiness(eventInterval) {
  return (
    eventInterval.source === 'PodState' &&
    (eventInterval.message.reason === 'Ready' ||
      eventInterval.message.reason === 'NotReady')
  )
}

function isKubeletReadinessCheck(eventInterval) {
  return (
    eventInterval.source === 'PodState' &&
    (eventInterval.message.reason === 'ReadinessFailed' ||
      eventInterval.message.reason === 'ReadinessErrored')
  )
}

function isKubeletStartupProbeFailure(eventInterval) {
  return (
    eventInterval.source === 'PodState' &&
    eventInterval.message.reason === 'StartupProbeFailed'
  )
}

function isE2EFailed(eventInterval) {
  if (
    eventInterval.source === 'E2ETest' &&
    eventInterval.message.annotations['status'] === 'Failed'
  ) {
    return true
  }
  return false
}

function isE2EFlaked(eventInterval) {
  if (
    eventInterval.source === 'E2ETest' &&
    eventInterval.message.annotations['status'] === 'Flaked'
  ) {
    return true
  }
  return false
}

function isE2EPassed(eventInterval) {
  if (
    eventInterval.source === 'E2ETest' &&
    eventInterval.message.annotations['status'] === 'Passed'
  ) {
    return true
  }
  return false
}

function isGracefulShutdownActivity(eventInterval) {
  return eventInterval.source === 'APIServerGracefulShutdown'
}

function isEndpointConnectivity(eventInterval) {
  if (
    eventInterval.message.reason !== 'DisruptionBegan' &&
    eventInterval.message.reason !== 'DisruptionSamplerOutageBegan'
  ) {
    return false
  }
  if (eventInterval.source === 'Disruption') {
    return true
  }
  if (
    eventInterval.locator.keys['namespace'] === 'e2e-k8s-service-lb-available'
  ) {
    return true
  }
  if (eventInterval.locator.keys.has('route')) {
    return true
  }

  return false
}

function isNodeState(eventInterval) {
  return eventInterval.source === 'NodeState'
}

function isCloudMetrics(eventInterval) {
  return eventInterval.source === 'CloudMetrics'
}

function isAlert(eventInterval) {
  return eventInterval.source === 'Alert'
}

function interestingEvents(item) {
  if (item.message.annotations['pathological'] === 'true') {
    if (item.message.annotations['interesting'] === 'true') {
      return [item.displayLocator, ` (pathological known)`, 'PathologicalKnown']
    } else {
      return [item.displayLocator, ` (pathological new)`, 'PathologicalNew']
    }
  }
  // TODO: hack that can likely be removed when we get to structured intervals for these
  // Always show pod sandbox events even if they didn't make it to pathological
  if (
    item.message.annotations['interesting'] === 'true' &&
    item.message.humanMessage.includes('pod sandbox')
  ) {
    return [item.displayLocator, ` (pod sandbox)`, 'PodSandbox']
  }

  if (item.message.includes('interesting/true')) {
    return [item.displayLocator, ` (interesting event)`, 'InterestingEvent']
  }
}

function podLogs(item) {
  if (item.level == 'Warning') {
    return [item.displayLocator, ` (pod log)`, 'PodLogWarning']
  }
  if (item.level == 'Error') {
    return [item.displayLocator, ` (pod log)`, 'PodLogError']
  }
  return [item.displayLocator, ` (pod log)`, 'PodLogInfo']
}

function journalLogs(item) {
  return [item.displayLocator, ` (system journal)`, 'SystemJournal']
}

const reReason = new RegExp('(^| )reason/([^ ]+)')
function podStateValue(item) {
  let m = item.message.match(reReason)

  if (m && isPodLifecycle(item)) {
    if (m[2] == 'Created') {
      return [item.displayLocator, ` (pod lifecycle)`, 'PodCreated']
    }
    if (m[2] == 'Scheduled') {
      return [item.displayLocator, ` (pod lifecycle)`, 'PodScheduled']
    }
    if (m[2] == 'GracefulDelete') {
      return [item.displayLocator, ` (pod lifecycle)`, 'PodTerminating']
    }
  }
  if (m && isContainerLifecycle(item)) {
    if (m[2] == 'ContainerWait') {
      return [item.displayLocator, ` (container lifecycle)`, 'ContainerWait']
    }
    if (m[2] == 'ContainerStart') {
      return [item.displayLocator, ` (container lifecycle)`, 'ContainerStart']
    }
  }
  if (m && isContainerReadiness(item)) {
    if (m[2] == 'NotReady') {
      return [
        item.displayLocator,
        ` (container readiness)`,
        'ContainerNotReady',
      ]
    }
    if (m[2] == 'Ready') {
      return [item.displayLocator, ` (container readiness)`, 'ContainerReady']
    }
  }
  if (m && isKubeletReadinessCheck(item)) {
    if (m[2] == 'ReadinessFailed') {
      return [
        item.displayLocator,
        ` (kubelet container readiness)`,
        'ContainerReadinessFailed',
      ]
    }
    if (m[2] == 'ReadinessErrored') {
      return [
        item.displayLocator,
        ` (kubelet container readiness)`,
        'ContainerReadinessErrored',
      ]
    }
  }
  if (m && isKubeletStartupProbeFailure(item)) {
    return [
      item.displayLocator,
      ` (kubelet container startupProbe)`,
      'StartupProbeFailed',
    ]
  }

  return [item.displayLocator, '', 'Unknown']
}

function nodeStateValue(item) {
  let roles = ''
  if (item.message.annotations.hasOwnProperty('roles')) {
    roles = item.message.annotations.roles
  }
  if (item.message.reason === 'NotReady') {
    return [item.displayLocator, ` (${roles})`, 'NodeNotReady']
  }
  let m = item.message.annotations.phase
  return [item.displayLocator, ` (${roles})`, m]
}

function etcdLeadershipLogsValue(item) {
  // If source is isEtcdLeadership, the term is always there.
  const term = item.message.annotations['term']

  // We are only charting the intervals with a node.
  const nodeVal = item.locator.keys['node']

  // Get etcd-member value (this will be present for a leader change).
  let etcdMemberVal = item.locator.keys['etcd-member'] || ''
  if (etcdMemberVal.length > 0) {
    etcdMemberVal = `etcd-member/${etcdMemberVal} `
  }

  let reason = item.message.reason
  let color = 'EtcdOther'
  if (reason.length > 0) {
    color = reason
    reason = `reason/${reason}`
  }
  return [`node/${nodeVal} ${etcdMemberVal} term/${term}`, ` ${reason}`, color]
}

function isEtcdLeadershipAndNotEmpty(item) {
  if (isEtcdLeadership(item)) {
    // Don't chart the ones where the node is empty.
    const node = item.locator.keys['node'] || ''
    if (node.length > 0) {
      return true
    }
  }
  return false
}

function cloudMetricsValue(item) {
  return [item.displayLocator, '', 'CloudMetric']
}

function alertSeverity(item) {
  // the other types can be pending, so check pending first
  if (item.message.annotations['alertstate'] === 'pending') {
    return [item.displayLocator, '', 'AlertPending']
  }

  if (item.message.annotations['severity'] === 'info') {
    return [item.displayLocator, '', 'AlertInfo']
  }
  if (item.message.annotations['severity'] === 'warning') {
    return [item.displayLocator, '', 'AlertWarning']
  }
  if (item.message.annotations['severity'] === 'critical') {
    return [item.displayLocator, '', 'AlertCritical']
  }

  // color as critical if nothing matches so that we notice that something has gone wrong
  return [item.displayLocator, '', 'AlertCritical']
}

function apiserverDisruptionValue(item) {
  // TODO: isolate DNS error into CIClusterDisruption
  return [item.displayLocator, '', 'Disruption']
}

function apiserverShutdownValue(item) {
  // TODO: isolate DNS error into CIClusterDisruption
  return [item.displayLocator, '', 'GracefulShutdownInterval']
}

function disruptionValue(item) {
  // We classify these disruption samples with this message if it thinks
  // it looks like a problem in the CI cluster running the tests, not the cluster under test.
  // (typically DNS lookup problems)
  let ciClusterDisruption = item.message.humanMessage.indexOf(
    'likely a problem in cluster running tests'
  )
  if (ciClusterDisruption != -1) {
    return [item.displayLocator, '', 'CIClusterDisruption']
  }
  return [item.displayLocator, '', 'Disruption']
}

function apiserverShutdownEventsValue(item) {
  // TODO: isolate DNS error into CIClusterDisruption
  return [item.displayLocator, '', 'GracefulShutdownWindow']
}

function getDurationString(durationSeconds) {
  const seconds = durationSeconds % 60
  const minutes = Math.floor(durationSeconds / 60)
  let durationString = '['
  if (minutes !== 0) {
    durationString += minutes + 'm'
  }
  durationString += seconds + 's]'
  return durationString
}

function defaultToolTip(item) {
  if (!item.message.annotations) {
    return ''
  }

  const structuredMessage = item.message
  const annotations = structuredMessage.annotations

  const keyValuePairs = Object.entries(annotations).map(([key, value]) => {
    return `${key}/${value}`
  })

  let tt = keyValuePairs.join(' ') + ' ' + structuredMessage.humanMessage

  // TODO: can probably remove this once we're confident all displayed intervals have it set
  if ('display' in item) {
    tt = 'display/' + item.display + ' ' + tt
  }
  if ('source' in item) {
    tt = 'source/' + item.source + ' ' + tt
  }
  tt =
    tt +
    ' ' +
    getDurationString(
      (new Date(item.to).getTime() - new Date(item.from).getTime()) / 1000
    )
  return tt
}

// Used for the actual locators displayed on the right hand side of the chart. Based on the origin go code that does
// similar for whenever we serialize a locator to display.
function buildLocatorDisplayString(i) {
  let keys = Object.keys(i.keys)
  keys = sortKeys(keys)

  let annotations = []
  for (let k of keys) {
    let v = i.keys[k]
    if (k === 'LocatorE2ETestKey') {
      annotations.push(`${k}/${JSON.stringify(v)}`)
    } else {
      annotations.push(`${k}/${v}`)
    }
  }

  return annotations.join(' ')
}

function sortKeys(keys) {
  // Ensure these keys appear in this order. Other keys can be mixed in and will appear at the end in alphabetical order.
  const orderedKeys = [
    'namespace',
    'node',
    'pod',
    'uid',
    'server',
    'container',
    'shutdown',
    'row',
  ]

  // Create a map to store the indices of keys in the orderedKeys array.
  // This will allow us to efficiently check if a key is in orderedKeys and find its position.
  const orderedKeyIndices = {}
  orderedKeys.forEach((key, index) => {
    orderedKeyIndices[key] = index
  })

  // Define a custom sorting function that orders the keys based on the orderedKeys array.
  keys.sort((a, b) => {
    // Get the indices of keys a and b in orderedKeys.
    const indexA = orderedKeyIndices[a]
    const indexB = orderedKeyIndices[b]

    // If both keys exist in orderedKeys, sort them based on their order.
    if (indexA !== undefined && indexB !== undefined) {
      return indexA - indexB
    }

    // If only one of the keys exists in orderedKeys, move it to the front.
    if (indexA !== undefined) {
      return -1
    } else if (indexB !== undefined) {
      return 1
    }

    // If neither key is in orderedKeys, sort alphabetically so we have predictable ordering.
    return a.localeCompare(b)
  })

  return keys
}

function createTimelineData(
  timelineVal,
  timelineData,
  filteredEventIntervals,
  source
) {
  const data = {}
  let now = new Date()
  let earliest = filteredEventIntervals.reduce(
    (accumulator, currentValue) =>
      !currentValue.from || accumulator < new Date(currentValue.from)
        ? accumulator
        : new Date(currentValue.from),
    new Date(now.getTime() + 1)
  )
  let latest = filteredEventIntervals.reduce(
    (accumulator, currentValue) =>
      !currentValue.to || accumulator > new Date(currentValue.to)
        ? accumulator
        : new Date(currentValue.to),
    new Date(now.getTime() - 1)
  )
  filteredEventIntervals.forEach((item) => {
    if (item.source !== source || !item.display) {
      return
    }
    let startDate = new Date(item.from)
    if (!item.from) {
      startDate = earliest
    }
    let endDate = new Date(item.to)
    if (!item.to) {
      endDate = latest
    }
    let label = buildLocatorDisplayString(item.locator)
    let sub = ''
    let val = timelineVal
    if (typeof val === 'function') {
      ;[label, sub, val] = timelineVal(item)
    }
    let section = data[label]
    if (!section) {
      section = {}
      data[label] = section
    }
    let ranges = section[sub]
    if (!ranges) {
      ranges = []
      section[sub] = ranges
    }
    ranges.push({
      timeRange: [startDate, endDate],
      val: val,
      labelVal: item.displayMessage,
    })
  })
  for (const label in data) {
    const section = data[label]
    for (const sub in section) {
      const data = section[sub]
      const totalDurationSeconds = data.reduce(
        (prev, curr) =>
          prev +
          (curr.timeRange[1].getTime() - curr.timeRange[0].getTime()) / 1000,
        0
      )

      timelineData.push({
        label: label + sub + ' ' + getDurationString(totalDurationSeconds),
        data: data,
      })
    }
  }
}
