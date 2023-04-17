import * as lodash from 'lodash'
import { Error } from '@material-ui/icons'
import Alert from '@material-ui/lab/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

export default function ProwJobRun(props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [eventIntervals, setEventIntervals] = React.useState([])

  const fetchData = () => {
    let queryString = ''
    console.log('hello world we got the prow job run id of ' + props.jobRunID)

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/jobs/runs/intervals?prow_job_run_id=' +
        props.jobRunID +
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
          setEventIntervals(json.items)
        } else {
          setEventIntervals([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve intervals for ' + props.jobRunID + ', ' + error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading intervals for job run {props.jobRunID}...</p>
  }

  // Structure the locator data and then categorize the event
  lodash.forEach(eventIntervals, function (eventInterval) {
    // break the locator apart into an object for better filtering
    eventInterval.locatorObj = {}
    if (eventInterval.locator.startsWith('e2e-test/')) {
      eventInterval.locatorObj.e2e_test = eventInterval.locator.slice(
        eventInterval.locator.indexOf('/') + 1
      )
    } else {
      let locatorChunks = eventInterval.locator.split(' ')
      _.forEach(locatorChunks, function (chunk) {
        let key = chunk.slice(0, chunk.indexOf('/'))
        eventInterval.locatorObj[key] = chunk.slice(chunk.indexOf('/') + 1)
      })
    }

    // TODO Wasn't clear if an event is only supposed to be in one category or if it can show up in multiple, with the existing implementation
    // it can show up more than once if it passes more than one of the category checks. If it is meant to only be one category this
    // could be something simpler like eventInterval.category = "operator-degraded" instead.
    // Not hypthetical, found events that passed isPodLogs also passed isPods.

    // Categorizing the events once on page load will save time on filtering later
    eventInterval.categories = {}
    let categorized = false
    eventInterval.categories.operator_unavailable =
      isOperatorAvailable(eventInterval)
    eventInterval.categories.operator_progressing =
      isOperatorProgressing(eventInterval)
    eventInterval.categories.operator_degraded =
      isOperatorDegraded(eventInterval)
    eventInterval.categories.pods = isPod(eventInterval)
    eventInterval.categories.pod_logs = isPodLog(eventInterval)
    eventInterval.categories.interesting_events =
      isInterestingOrPathological(eventInterval)
    eventInterval.categories.alerts = isAlert(eventInterval)
    eventInterval.categories.node_state = isNodeState(eventInterval)
    eventInterval.categories.e2e_test_failed = isE2EFailed(eventInterval)
    eventInterval.categories.e2e_test_flaked = isE2EFlaked(eventInterval)
    eventInterval.categories.e2e_test_passed = isE2EPassed(eventInterval)
    eventInterval.categories.endpoint_availability =
      isEndpointConnectivity(eventInterval)
    eventInterval.categories.uncategorized = !_.some(eventInterval.categories) // will save time later during filtering and re-rendering since we don't render any uncategorized events
  })

  return (
    /* eslint-disable react/prop-types */
    <Fragment>
      <p>Loaded {eventIntervals.length} intervals</p>

      <div id="chart"></div>
    </Fragment>
  )
}

ProwJobRun.defaultProps = {}

ProwJobRun.propTypes = {
  jobRunID: PropTypes.string.isRequired,
  filterModel: PropTypes.object,
}

function isOperatorAvailable(eventInterval) {
  if (
    eventInterval.locator.startsWith('clusteroperator/') &&
    eventInterval.message.includes('condition/Available') &&
    eventInterval.message.includes('status/False')
  ) {
    return true
  }
  return false
}

function isOperatorDegraded(eventInterval) {
  if (
    eventInterval.locator.startsWith('clusteroperator/') &&
    eventInterval.message.includes('condition/Degraded') &&
    eventInterval.message.includes('status/True')
  ) {
    return true
  }
  return false
}

function isOperatorProgressing(eventInterval) {
  if (
    eventInterval.locator.startsWith('clusteroperator/') &&
    eventInterval.message.includes('condition/Progressing') &&
    eventInterval.message.includes('status/True')
  ) {
    return true
  }
  return false
}

function isPodLog(eventInterval) {
  if (eventInterval.locator.includes('src/podLog')) {
    return true
  }
  return false
}

function isInterestingOrPathological(eventInterval) {
  if (
    eventInterval.message.includes('pathological/true') ||
    eventInterval.message.includes('interesting/true')
  ) {
    return true
  }
  return false
}

function isPod(eventInterval) {
  // this check was added to keep the repeating events out fo the "pods" section
  const nTimes = new RegExp('\\(\\d+ times\\)')
  if (eventInterval.message.match(nTimes)) {
    return false
  }
  // this check was added to avoid the events from the "interesting-events" section from being
  // duplicated in the "pods" section.
  if (isInterestingOrPathological(eventInterval)) {
    return false
  }
  if (
    eventInterval.locator.includes('pod/') &&
    !eventInterval.locator.includes('alert/')
  ) {
    return true
  }
  return false
}

function isPodLifecycle(eventInterval) {
  if (
    eventInterval.locator.includes('pod/') &&
    (eventInterval.message.includes('reason/Created') ||
      eventInterval.message.includes('reason/Scheduled') ||
      eventInterval.message.includes('reason/GracefulDelete'))
  ) {
    return true
  }
  return false
}

function isContainerLifecycle(eventInterval) {
  if (
    eventInterval.locator.includes('container/') &&
    (eventInterval.message.includes('reason/ContainerExit') ||
      eventInterval.message.includes('reason/ContainerStart') ||
      eventInterval.message.includes('reason/ContainerWait'))
  ) {
    return true
  }
  return false
}

function isContainerReadiness(eventInterval) {
  if (
    eventInterval.locator.includes('container/') &&
    (eventInterval.message.includes('reason/Ready') ||
      eventInterval.message.includes('reason/NotReady'))
  ) {
    return true
  }
  return false
}

function isKubeletReadinessCheck(eventInterval) {
  if (
    eventInterval.locator.includes('container/') &&
    (eventInterval.message.includes('reason/ReadinessFailed') ||
      eventInterval.message.includes('reason/ReadinessErrored'))
  ) {
    return true
  }
  return false
}

function isKubeletStartupProbeFailure(eventInterval) {
  if (
    eventInterval.locator.includes('container/') &&
    eventInterval.message.includes('reason/StartupProbeFailed')
  ) {
    return true
  }
  return false
}

function isE2EFailed(eventInterval) {
  if (
    eventInterval.locator.startsWith('e2e-test/') &&
    eventInterval.message.includes('finished As "Failed')
  ) {
    return true
  }
  return false
}

function isE2EFlaked(eventInterval) {
  if (
    eventInterval.locator.startsWith('e2e-test/') &&
    eventInterval.message.includes('finished As "Flaked')
  ) {
    return true
  }
  return false
}

function isE2EPassed(eventInterval) {
  if (
    eventInterval.locator.startsWith('e2e-test/') &&
    eventInterval.message.includes('finished As "Passed')
  ) {
    return true
  }
  return false
}

function isEndpointConnectivity(eventInterval) {
  if (
    !eventInterval.message.includes('reason/DisruptionBegan') &&
    !eventInterval.message.includes('reason/DisruptionSamplerOutageBegan')
  ) {
    return false
  }
  if (eventInterval.locator.includes('disruption/')) {
    return true
  }
  if (eventInterval.locator.startsWith('ns/e2e-k8s-service-lb-available')) {
    return true
  }
  if (eventInterval.locator.includes(' route/')) {
    return true
  }

  return false
}

function isNodeState(eventInterval) {
  if (eventInterval.locator.startsWith('node/')) {
    return (
      eventInterval.message.startsWith('reason/NodeUpdate ') ||
      eventInterval.message.includes('node is not ready')
    )
  }
  return false
}

function isAlert(eventInterval) {
  if (eventInterval.locator.startsWith('alert/')) {
    return true
  }
  return false
}

function interestingEvents(item) {
  if (item.message.includes('pathological/true')) {
    if (item.message.includes('interesting/true')) {
      return [item.locator, ` (pathological known)`, 'PathologicalKnown']
    } else {
      return [item.locator, ` (pathological new)`, 'PathologicalNew']
    }
  }
  if (item.message.includes('interesting/true')) {
    return [item.locator, ` (interesting event)`, 'InterestingEvent']
  }
}

function podLogs(item) {
  if (item.level == 'Warning') {
    return [item.locator, ` (pod log)`, 'PodLogWarning']
  }
  if (item.level == 'Error') {
    return [item.locator, ` (pod log)`, 'PodLogError']
  }
  return [item.locator, ` (pod log)`, 'PodLogInfo']
}

const reReason = new RegExp('(^| )reason/([^ ]+)')
function podStateValue(item) {
  let m = item.message.match(reReason)

  if (m && isPodLifecycle(item)) {
    if (m[2] == 'Created') {
      return [item.locator, ` (pod lifecycle)`, 'PodCreated']
    }
    if (m[2] == 'Scheduled') {
      return [item.locator, ` (pod lifecycle)`, 'PodScheduled']
    }
    if (m[2] == 'GracefulDelete') {
      return [item.locator, ` (pod lifecycle)`, 'PodTerminating']
    }
  }
  if (m && isContainerLifecycle(item)) {
    if (m[2] == 'ContainerWait') {
      return [item.locator, ` (container lifecycle)`, 'ContainerWait']
    }
    if (m[2] == 'ContainerStart') {
      return [item.locator, ` (container lifecycle)`, 'ContainerStart']
    }
  }
  if (m && isContainerReadiness(item)) {
    if (m[2] == 'NotReady') {
      return [item.locator, ` (container readiness)`, 'ContainerNotReady']
    }
    if (m[2] == 'Ready') {
      return [item.locator, ` (container readiness)`, 'ContainerReady']
    }
  }
  if (m && isKubeletReadinessCheck(item)) {
    if (m[2] == 'ReadinessFailed') {
      return [
        item.locator,
        ` (kubelet container readiness)`,
        'ContainerReadinessFailed',
      ]
    }
    if (m[2] == 'ReadinessErrored') {
      return [
        item.locator,
        ` (kubelet container readiness)`,
        'ContainerReadinessErrored',
      ]
    }
  }
  if (m && isKubeletStartupProbeFailure(item)) {
    return [
      item.locator,
      ` (kubelet container startupProbe)`,
      'StartupProbeFailed',
    ]
  }

  return [item.locator, '', 'Unknown']
}

const rePhase = new RegExp('(^| )phase/([^ ]+)')
function nodeStateValue(item) {
  let roles = ''
  let i = item.message.indexOf('roles/')
  if (i != -1) {
    roles = item.message.substring(i + 'roles/'.length)
    let j = roles.indexOf(' ')
    if (j != -1) {
      roles = roles.substring(0, j)
    }
  }

  if (item.message.includes('node is not ready')) {
    return [item.locator, ` (${roles},not ready)`, 'NodeNotReady']
  }
  let m = item.message.match(rePhase)
  if (m && m[2] != 'Update') {
    return [item.locator, ` (${roles},update phases)`, m[2]]
  }
  return [item.locator, ` (${roles},updates)`, 'Update']
}

function alertSeverity(item) {
  // the other types can be pending, so check pending first
  let pendingIndex = item.message.indexOf('pending')
  if (pendingIndex != -1) {
    return [item.locator, '', 'AlertPending']
  }

  let infoIndex = item.message.indexOf('info')
  if (infoIndex != -1) {
    return [item.locator, '', 'AlertInfo']
  }
  let warningIndex = item.message.indexOf('warning')
  if (warningIndex != -1) {
    return [item.locator, '', 'AlertWarning']
  }
  let criticalIndex = item.message.indexOf('critical')
  if (criticalIndex != -1) {
    return [item.locator, '', 'AlertCritical']
  }

  // color as critical if nothing matches so that we notice that something has gone wrong
  return [item.locator, '', 'AlertCritical']
}

function disruptionValue(item) {
  // We classify these disruption samples with this message if it thinks
  // it looks like a problem in the CI cluster running the tests, not the cluster under test.
  // (typically DNS lookup problems)
  let ciClusterDisruption = item.message.indexOf(
    'likely a problem in cluster running tests'
  )
  if (ciClusterDisruption != -1) {
    return [item.locator, '', 'CIClusterDisruption']
  }
  return [item.locator, '', 'Disruption']
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
  return (
    item.message +
    ' ' +
    getDurationString(
      (new Date(item.to).getTime() - new Date(item.from).getTime()) / 1000
    )
  )
}
function createTimelineData(
  timelineVal,
  timelineData,
  filteredEventIntervals,
  category
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
    if (!item.categories[category]) {
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
    let label = item.locator
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
      labelVal: defaultToolTip(item),
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
