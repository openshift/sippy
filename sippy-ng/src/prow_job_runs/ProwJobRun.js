import * as lodash from 'lodash'
import { Button, ButtonGroup, TextField } from '@material-ui/core'
import { Error } from '@material-ui/icons'
import { useHistory } from 'react-router-dom'
import Alert from '@material-ui/lab/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect, useState } from 'react'
import TimelineChart from '../components/TimelineChart'

export default function ProwJobRun(props) {
  const history = useHistory()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [eventIntervals, setEventIntervals] = React.useState([])

  // TODO: Not sure this needs to be a useState param, really just need a constant map but
  // wasn't working.
  const [allCategories, setAllCategories] = useState(
    new Map([
      ['operator_unavailable', 'Operator Unavailable'],
      ['operator_progressing', 'Operator Progressing'],
      ['operator_degraded', 'Operator Degraded'],
      ['pods', 'Pods (careful)'],
      ['pod_logs', 'Pod Logs'],
      ['interesting_events', 'Interesting Events'],
      ['alerts', 'Alerts'],
      ['node_state', 'Node State'],
      ['e2e_test_failed', 'E2E Failed'],
      ['e2e_test_flaked', 'E2E Flaked'],
      ['e2e_test_passed', 'E2E Passed'],
      ['disruption', 'Disruption'],
    ])
  )

  const [categoryButtonState, setCategoryButtonState] = useState(() => {
    const params = new URLSearchParams(window.location.search)

    let categories = []
    let categoriesParam = params.get('categories')
    if (categoriesParam) {
      categories = categoriesParam.split(',')
    }

    const initialState = {}
    let atLeastOneCategorySelected = false
    allCategories.forEach((value, key, map) => {
      initialState[key] = categories.includes(key)
      if (categories.includes(key)) {
        atLeastOneCategorySelected = true
      }
    })

    if (!atLeastOneCategorySelected) {
      console.log('no category was selected, enabling the default set')
      initialState['operator_unavailable'] = true
      initialState['operator_progressing'] = true
      initialState['operator_degraded'] = true
      initialState['pod_logs'] = true
      initialState['interesting_events'] = true
      initialState['alerts'] = true
      initialState['node_state'] = true
      initialState['e2e_test_failed'] = true
      initialState['disruption'] = true
    }

    // If no buttons were selected, you're not going to see anything, and this likely implies
    // you are fresh loading the page. Select the default set.
    console.log('calculated initial category button state:')
    console.log(initialState)
    return initialState
  })

  const [intervalFiles, setIntervalFiles] = useState([])
  const [selectedIntervalFiles, setSelectedIntervalFiles] = useState(() => {
    const params = new URLSearchParams(window.location.search)
    let tempFiles = []
    let p = params.get('intervalFiles')
    if (p) {
      tempFiles = p.split(',')
    }
    return tempFiles
  })

  const [filterText, setFilterText] = useState(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('filter')) {
      return params.get('filter')
    }
    return ''
  })

  const fetchData = () => {
    if (isLoaded) {
      console.log(
        'fetchData called due to UI update but intervals already loaded, re-using intervals'
      )
      return
    }
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
          // Process and filter our intervals
          let tmpIntervals = json.items
          mutateIntervals(tmpIntervals)
          setEventIntervals(tmpIntervals)

          let newEventIntervalFiles = []
          lodash.forEach(tmpIntervals, function (eventInterval) {
            if (!newEventIntervalFiles.includes(eventInterval.filename)) {
              newEventIntervalFiles.push(eventInterval.filename)
            }
          })
          console.log('newEventIntervalFiles = ' + newEventIntervalFiles)
          newEventIntervalFiles.sort()
          setIntervalFiles(newEventIntervalFiles)

          // On initial load, use the first file as the selected interval file:
          if (selectedIntervalFiles.length === 0) {
            setSelectedIntervalFiles([newEventIntervalFiles[0]])
          }
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

    // Update the URL query params whenever button state changes:
    const queryParams = new URLSearchParams()
    let categories = []
    Object.keys(categoryButtonState).forEach(function (button) {
      if (categoryButtonState[button]) {
        categories.push(button)
      }
    })
    queryParams.set('categories', categories.join(','))

    queryParams.set('intervalFiles', selectedIntervalFiles.join(','))

    // Delayed processing of the filter text input to allow the user to finish typing before
    // we execute a search/filter:
    const timer = setTimeout(() => {
      console.log('Filter text updated:', filterText)
      // TODO: do something with the input
      // TODO: the delay is not working, we trigger filtering immediately when typing I think
      // because the filterIntervals call is outside any useEffect.
    }, 1000)
    if (!(filterText === '')) {
      queryParams.set('filter', filterText)
    }

    console.log('origin: ' + window.location.origin)
    console.log('pathname: ' + window.location.pathname)
    console.log('params: ' + queryParams.toString())
    history.replace({
      search: queryParams.toString(),
    })

    return () => clearTimeout(timer)
  }, [categoryButtonState, history, selectedIntervalFiles, filterText])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading intervals for job run {props.jobRunID}...</p>
  }

  let filteredIntervals = filterIntervals(
    eventIntervals,
    categoryButtonState,
    selectedIntervalFiles,
    filterText
  )

  let chartData = groupIntervals(filteredIntervals)

  function handleCategoryClick(buttonValue) {
    console.log('got category button click: ' + buttonValue)
    setCategoryButtonState((prevState) => ({
      ...prevState,
      [buttonValue]: !prevState[buttonValue],
    }))
    console.log(categoryButtonState)
  }

  function handleIntervalFileClick(buttonValue) {
    console.log('got interval file button click: ' + buttonValue)
    const newSelectedIntervalFiles = [...selectedIntervalFiles]
    const selectedIndex = selectedIntervalFiles.indexOf(buttonValue)

    if (selectedIndex === -1) {
      console.log(buttonValue + ' is now selected')
      newSelectedIntervalFiles.push(buttonValue)
    } else {
      console.log(buttonValue + ' is no longer selected')
      newSelectedIntervalFiles.splice(selectedIndex, 1)
    }

    console.log('new selected interval files: ' + newSelectedIntervalFiles)
    setSelectedIntervalFiles(newSelectedIntervalFiles)
  }

  const handleFilterChange = (event) => {
    setFilterText(event.target.value)
  }

  return (
    /* eslint-disable react/prop-types */
    <Fragment>
      <p>
        Loaded {eventIntervals.length} intervals from GCS, filtered down to{' '}
        {filteredIntervals.length}.
      </p>
      <p>
        Categories:
        <ButtonGroup size="small" aria-label="Categories">
          {[...allCategories.entries()].map(([key, value]) => (
            <Button
              key={key}
              onClick={() => handleCategoryClick(key)}
              variant={categoryButtonState[key] ? 'contained' : 'outlined'}
            >
              {value}
            </Button>
          ))}
        </ButtonGroup>
      </p>
      <p>
        Files:
        <ButtonGroup size="small" aria-label="Categories">
          {intervalFiles.map((intervalFile) => (
            <Button
              key={intervalFile}
              onClick={() => handleIntervalFileClick(intervalFile)}
              variant={
                selectedIntervalFiles.includes(intervalFile)
                  ? 'contained'
                  : 'outlined'
              }
            >
              {intervalFile}
            </Button>
          ))}
        </ButtonGroup>
      </p>
      <p>
        <TextField
          id="filter"
          label="Regex Filter"
          variant="outlined"
          onChange={handleFilterChange}
        />
      </p>
      <TimelineChart data={chartData} eventIntervals={filteredIntervals} />
    </Fragment>
  )
}

ProwJobRun.defaultProps = {}

ProwJobRun.propTypes = {
  jobRunID: PropTypes.string.isRequired,
  filterModel: PropTypes.object,
}

function filterIntervals(
  eventIntervals,
  categoryButtonState,
  selectedIntervalFiles,
  filterText
) {
  let isSet = false
  /*
  let positiveSelectionRows = new Map()
  let negativeSelectionRows = new Map()
  for (let i = 0; i < numPositiveSelectors; i++) {
    var not = $('#not_' + i).is(':checked')
    var category = $('#category_' + i).val()
    var lodash = $('#lodash_' + i).val()
    var ns = $('#namespace_' + i).val()
    var regexStr = $('#filterInput_' + i).val()
    var currIsSet =
      regexStr.length != 0 ||
      lodash.length != 0 ||
      ns.length != 0 ||
      category.length != 0
    console.log('value of not ' + not)
    if (not) {
      negativeSelectionRows.set(i, {
        isSet: currIsSet,
        lodash: lodash,
        regexStr: regexStr,
        regex: new RegExp(regexStr),
        namespace: ns,
        category: category,
      })
    } else {
      positiveSelectionRows.set(i, {
        isSet: currIsSet,
        lodash: lodash,
        regexStr: regexStr,
        regex: new RegExp(regexStr),
        namespace: ns,
        category: category,
      })
    }

    if (currIsSet) {
      isSet = true
    }
  }
   */

  // if none of the filter inputs are set, nothing to filter so don't waste time looping through everything
  //let filteredIntervals = eventIntervals

  let re = null
  if (filterText) {
    re = new RegExp(filterText)
  }

  return _.filter(eventIntervals, function (eventInterval) {
    if (!selectedIntervalFiles.includes(eventInterval.filename)) {
      return false
    }
    let shouldInclude = false
    // Go ahead and filter out uncategorized events
    Object.keys(eventInterval.categories).forEach(function (cat) {
      if (eventInterval.categories[cat] && categoryButtonState[cat]) {
        if (re) {
          if (
            re.test(eventInterval.message) ||
            re.test(eventInterval.locator)
          ) {
            shouldInclude = true
          }
        } else {
          shouldInclude = true
        }
      }
    })
    return shouldInclude
  })

  /*
if (isSet) {
// At least one of the inputs had a value, test any inputs that had values
// This currently does an OR operation of the input fields
filteredIntervals = _.filter(eventIntervals, function (eventInterval) {
  // Go ahead and filter out uncategorized events
  if (eventInterval.categories.uncategorized) {
    return false
  }

  let eventMatches = false

  for (let [key, positiveSelectionRow] of positiveSelectionRows) {
    if (positiveSelectionRow.isSet) {
      matchRegex =
        positiveSelectionRow.regexStr.length == 0 ||
        positiveSelectionRow.regex.test(eventInterval.locator)
      matchNS =
        positiveSelectionRow.namespace.length == 0 ||
        (eventInterval.locatorObj.ns &&
          eventInterval.locatorObj.ns.includes(
            positiveSelectionRow.namespace
          ))
      matchCategory =
        positiveSelectionRow.category.length == 0 ||
        eventInterval.categories[positiveSelectionRow.category]
      matchLodash = true
      if (positiveSelectionRow.lodash.length > 0) {
        lodashMatches = _.filter(
          [eventInterval],
          _.matches(JSON.parse(positiveSelectionRow.lodash))
        )
        matchLodash = lodashMatches.length > 0
      }
      matchesPositive =
        matchRegex && matchLodash && matchNS && matchCategory

      if (matchesPositive) {
        console.log('matched positive ' + negativeSelectionRows.size)
        var exclude = false
        for (let [key, negativeSelectionRow] of negativeSelectionRows) {
          if (negativeSelectionRow.isSet) {
            console.log('checking negative')
            matchRegex =
              negativeSelectionRow.regexStr.length == 0 ||
              negativeSelectionRow.regex.test(eventInterval.locator)
            matchNS =
              negativeSelectionRow.namespace.length == 0 ||
              (eventInterval.locatorObj.ns &&
                eventInterval.locatorObj.ns.includes(
                  negativeSelectionRow.namespace
                ))
            matchCategory =
              negativeSelectionRow.category.length == 0 ||
              eventInterval.categories[negativeSelectionRow.category]
            matchLodash = true
            if (negativeSelectionRow.lodash.length > 0) {
              lodashMatches = _.filter(
                [eventInterval],
                _.matches(JSON.parse(negativeSelectionRow.lodash))
              )
              matchLodash = lodashMatches.length > 0
            }
            matchesNegative =
              matchRegex && matchLodash && matchNS && matchCategory

            if (matchesNegative) {
              exclude = true
              break
            }
          }
        }


        if (exclude) {
        } else {
          eventMatches = true
          break
        }
      }
    }
  }

      return eventMatches
    })
  }
   */
}

function mutateIntervals(eventIntervals) {
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
    eventInterval.categories.disruption = isEndpointConnectivity(eventInterval)
    eventInterval.categories.uncategorized = !_.some(eventInterval.categories) // will save time later during filtering and re-rendering since we don't render any uncategorized events
  })
}

function groupIntervals(filteredIntervals) {
  let timelineGroups = []
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
  timelineGroups[timelineGroups.length - 1].data.sort(function (e1, e2) {
    if (e1.label.includes('master') && e2.label.includes('worker')) {
      return -1
    }
    return 0
  })

  timelineGroups.push({ group: 'disruption', data: [] })
  createTimelineData(
    disruptionValue,
    timelineGroups[timelineGroups.length - 1].data,
    filteredIntervals,
    'disruption'
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
  return timelineGroups
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
