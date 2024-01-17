import {
  ArrayParam,
  BooleanParam,
  NumberParam,
  SafeStringParam,
  StringParam,
  useQueryParam,
} from 'use-query-params'
import {
  dateEndFormat,
  dateFormat,
  formatLongDate,
  getAPIUrl,
  gotFetchError,
} from './CompReadyUtils'
import { ReleasesContext } from '../App'
import { safeEncodeURIComponent } from '../helpers'
import CompReadyProgress from './CompReadyProgress'
import PropTypes from 'prop-types'
import React, { createContext, useContext, useEffect, useState } from 'react'
export const CompReadyVarsContext = createContext()

export const CompReadyVarsProvider = ({ children }) => {
  const [excludeNetworksList, setExcludeNetworksList] = useState([])
  const [excludeCloudsList, setExcludeCloudsList] = useState([])
  const [excludeArchesList, setExcludeArchesList] = useState([])
  const [excludeUpgradesList, setExcludeUpgradesList] = useState([])
  const [excludeVariantsList, setExcludeVariantsList] = useState([])
  const [isLoaded, setIsLoaded] = useState(false)
  const [fetchError, setFetchError] = useState('')

  const releases = useContext(ReleasesContext)

  // Find the most recent GA
  const gaReleases = Object.keys(releases.ga_dates)
  gaReleases.sort(
    (a, b) => new Date(releases.ga_dates[b]) - new Date(releases.ga_dates[a])
  )
  const defaultBaseRelease = gaReleases[0]

  // Find the release after that
  const nextReleaseIndex = releases.releases.indexOf(defaultBaseRelease) - 1
  const defaultSampleRelease = releases.releases[nextReleaseIndex]

  const getReleaseDate = (release) => {
    if (releases.ga_dates && releases.ga_dates[release]) {
      return new Date(releases.ga_dates[release])
    }

    return new Date()
  }
  const days = 24 * 60 * 60 * 1000
  const seconds = 1000
  const now = new Date()

  // Sample is last 7 days by default
  const initialSampleStartTime = new Date(now.getTime() - 6 * days)
  const initialSampleEndTime = new Date(now.getTime())

  // Base is 28 days from the default base release's GA date
  // Match what the metrics uses in the api.
  const initialBaseStartTime =
    getReleaseDate(defaultBaseRelease).getTime() - 27 * days
  const initialBaseEndTime =
    getReleaseDate(defaultBaseRelease).getTime() + 1 * days - 1 * seconds

  console.log('defaultBaseRelease: ', defaultBaseRelease)
  console.log(
    'initialBaseStartTime: ',
    formatLongDate(initialBaseStartTime, dateFormat)
  )
  console.log(
    'initialBaseEndTime: ',
    formatLongDate(initialBaseEndTime, dateEndFormat)
  )

  // Create the variables for the URL and set any initial values.
  const [baseReleaseParam = defaultBaseRelease, setBaseReleaseParam] =
    useQueryParam('baseRelease', StringParam)
  const [
    baseStartTimeParam = formatLongDate(initialBaseStartTime, dateFormat),
    setBaseStartTimeParam,
  ] = useQueryParam('baseStartTime', StringParam)
  const [
    baseEndTimeParam = formatLongDate(initialBaseEndTime, dateEndFormat),
    setBaseEndTimeParam,
  ] = useQueryParam('baseEndTime', StringParam)
  const [sampleReleaseParam = defaultSampleRelease, setSampleReleaseParam] =
    useQueryParam('sampleRelease', StringParam)
  const [
    sampleStartTimeParam = formatLongDate(initialSampleStartTime, dateFormat),
    setSampleStartTimeParam,
  ] = useQueryParam('sampleStartTime', StringParam)
  const [
    sampleEndTimeParam = formatLongDate(initialSampleEndTime, dateEndFormat),
    setSampleEndTimeParam,
  ] = useQueryParam('sampleEndTime', StringParam)
  const [
    groupByCheckedItemsParam = ['cloud', 'arch', 'network'],
    setGroupByCheckedItemsParam,
  ] = useQueryParam('groupBy', ArrayParam)
  const [
    excludeCloudsCheckedItemsParam = [
      'openstack',
      'ibmcloud',
      'libvirt',
      'ovirt',
      'unknown',
    ],
    setExcludeCloudsCheckedItemsParam,
  ] = useQueryParam('excludeClouds', ArrayParam)
  const [
    excludeArchesCheckedItemsParam = [
      'arm64',
      'heterogeneous',
      'ppc64le',
      's390x',
    ],
    setExcludeArchesCheckedItemsParam,
  ] = useQueryParam('excludeArches', ArrayParam)
  const [
    excludeNetworksCheckedItemsParam = [],
    setExcludeNetworksCheckedItemsParam,
  ] = useQueryParam('excludeNetworks', ArrayParam)
  const [
    excludeUpgradesCheckedItemsParam = [],
    setExcludeUpgradesCheckedItemsParam,
  ] = useQueryParam('excludeUpgrades', ArrayParam)
  const [
    excludeVariantsCheckedItemsParam = [
      'hypershift',
      'osd',
      'microshift',
      'techpreview',
      'single-node',
      'assisted',
      'compact',
    ],
    setExcludeVariantsCheckedItemsParam,
  ] = useQueryParam('excludeVariants', ArrayParam)

  const [confidenceParam = 95, setConfidenceParam] = useQueryParam(
    'confidence',
    NumberParam
  )
  const [pityParam = 5, setPityParam] = useQueryParam('pity', NumberParam)
  const [minFailParam = 3, setMinFailParam] = useQueryParam(
    'minFail',
    NumberParam
  )
  const [ignoreMissingParam = false, setIgnoreMissingParam] = useQueryParam(
    'ignoreMissing',
    BooleanParam
  )
  const [ignoreDisruptionParam = false, setIgnoreDisruptionParam] =
    useQueryParam('ignoreDisruption', BooleanParam)

  // Create the variables to be used for api calls; these are initilized to the
  // value of the variables that got their values from the URL.
  const [groupByCheckedItems, setGroupByCheckedItems] = React.useState(
    groupByCheckedItemsParam
  )

  const [componentParam, setComponentParam] = useQueryParam(
    'component',
    SafeStringParam
  )
  const [environmentParam, setEnvironmentParam] = useQueryParam(
    'environment',
    StringParam
  )
  const [capabilityParam, setCapabilityParam] = useQueryParam(
    'capability',
    StringParam
  )

  const [excludeCloudsCheckedItems, setExcludeCloudsCheckedItems] =
    React.useState(excludeCloudsCheckedItemsParam)
  const [excludeArchesCheckedItems, setExcludeArchesCheckedItems] =
    React.useState(excludeArchesCheckedItemsParam)
  const [excludeNetworksCheckedItems, setExcludeNetworksCheckedItems] =
    React.useState(excludeNetworksCheckedItemsParam)

  const [baseRelease, setBaseRelease] = React.useState(baseReleaseParam)

  const [sampleRelease, setSampleRelease] = React.useState(sampleReleaseParam)

  const [baseStartTime, setBaseStartTime] = React.useState(baseStartTimeParam)

  const [baseEndTime, setBaseEndTime] = React.useState(baseEndTimeParam)

  const [sampleStartTime, setSampleStartTime] =
    React.useState(sampleStartTimeParam)
  const [sampleEndTime, setSampleEndTime] = React.useState(sampleEndTimeParam)

  const setBaseReleaseWithDates = (event) => {
    let release = event.target.value
    let endTime = getReleaseDate(release)
    let startTime = endTime.getTime() - 30 * days
    setBaseRelease(release)
    setBaseStartTime(formatLongDate(startTime, dateFormat))
    setBaseEndTime(formatLongDate(endTime, dateEndFormat))
  }

  const setSampleReleaseWithDates = (event) => {
    let release = event.target.value
    setSampleRelease(release)
    setSampleStartTime(formatLongDate(initialSampleStartTime, dateFormat))
    setSampleEndTime(formatLongDate(initialSampleEndTime, dateEndFormat))
  }

  const [excludeUpgradesCheckedItems, setExcludeUpgradesCheckedItems] =
    React.useState(excludeUpgradesCheckedItemsParam)
  const [excludeVariantsCheckedItems, setExcludeVariantsCheckedItems] =
    React.useState(excludeVariantsCheckedItemsParam)

  const [confidence, setConfidence] = React.useState(confidenceParam)
  const [pity, setPity] = React.useState(pityParam)
  const [minFail, setMinFail] = React.useState(minFailParam)

  // for the two boolean values here, we need the || false because otherwise
  // the value will be null.
  const [ignoreMissing, setIgnoreMissing] = React.useState(
    ignoreMissingParam || false
  )
  const [ignoreDisruption, setIgnoreDisruption] = React.useState(
    ignoreDisruptionParam || true
  )

  const [component, setComponent] = React.useState(componentParam)
  if (component != componentParam) {
    setComponent(componentParam)
  }
  const [environment, setEnvironment] = React.useState(environmentParam)
  if (environment != environmentParam) {
    setEnvironment(environmentParam)
  }
  const [capability, setCapability] = React.useState(capabilityParam)
  if (capability != capabilityParam) {
    setCapability(capabilityParam)
  }

  // This runs when someone pushes the "Generate Report" button.
  // We form an api string and then call the api.
  const handleGenerateReport = (event) => {
    event.preventDefault()
    setBaseReleaseParam(baseRelease)
    setBaseStartTimeParam(formatLongDate(baseStartTime, dateFormat))
    setBaseEndTimeParam(formatLongDate(baseEndTime, dateEndFormat))
    setSampleReleaseParam(sampleRelease)
    setSampleStartTimeParam(formatLongDate(sampleStartTime, dateFormat))
    setSampleEndTimeParam(formatLongDate(sampleEndTime, dateEndFormat))
    setGroupByCheckedItemsParam(groupByCheckedItems)
    setExcludeCloudsCheckedItemsParam(excludeCloudsCheckedItems)
    setExcludeArchesCheckedItemsParam(excludeArchesCheckedItems)
    setExcludeNetworksCheckedItemsParam(excludeNetworksCheckedItems)
    setExcludeUpgradesCheckedItemsParam(excludeUpgradesCheckedItems)
    setExcludeVariantsCheckedItemsParam(excludeVariantsCheckedItems)
    setConfidenceParam(confidence)
    setPityParam(pity)
    setMinFailParam(minFail)
    setIgnoreDisruptionParam(ignoreDisruption)
    setIgnoreMissingParam(ignoreMissing)
    setComponentParam(component)
    setEnvironmentParam(environment)
    setCapabilityParam(capability)
  }

  useEffect(() => {
    const apiCallStr = getAPIUrl() + '/variants'
    fetch(apiCallStr)
      .then((response) => response.json())
      .then((data) => {
        if (data.code < 200 || data.code >= 300) {
          const errorMessage = data.message
            ? `${data.message}`
            : 'No error message'
          throw new Error(`Return code = ${data.code} (${errorMessage})`)
        }
        setExcludeCloudsList(data.platform)
        setExcludeArchesList(data.arch)
        setExcludeNetworksList(data.network)
        setExcludeUpgradesList(data.upgrade)
        setExcludeVariantsList(data.variant)
        setIsLoaded(true)
      })
      .catch((error) => {
        setFetchError(`API call failed: ${apiCallStr}\n${error}`)
      })
      .finally(() => {
        // Mark the attempt as finished whether successful or not.
        setIsLoaded(true)
      })
  }, [])

  // Take a string that is an "environment" (environment is a list of strings that describe
  // items in one or more of the lists above) and split it up so that it can be used in
  // an api call.  We keep this concept of "environment" because it's used for column labels.
  const expandEnvironment = (environmentStr) => {
    if (
      environmentStr == null ||
      environmentStr == '' ||
      environmentStr === 'No data'
    ) {
      return ''
    }
    const items = environmentStr.split(' ')
    const params = {}
    items.forEach((item) => {
      if (excludeCloudsList.includes(item)) {
        params.platform = item
      } else if (excludeArchesList.includes(item)) {
        params.arch = item
      } else if (excludeNetworksList.includes(item)) {
        params.network = item
      } else if (excludeUpgradesList.includes(item)) {
        params.upgrade = item
      } else if (excludeVariantsList.includes(item)) {
        params.variant = item
      } else {
        console.log(`Warning: Item '${item}' not found in lists`)
      }
    })
    const paramStrings = Object.entries(params).map(
      ([key, value]) => `${key}=${value}`
    )

    // We keep the environment along with the expanded environment for other components that
    // may use it.
    const safeEnvironment = safeEncodeURIComponent(environmentStr)
    const retVal =
      `&environment=${safeEnvironment}` + '&' + paramStrings.join('&')
    return retVal
  }

  const cancelFetch = () => {
    // This button will do nothing for now and may never need to
    // since the api call is very quick.
    console.log('Aborting /variant sippy API call')
  }

  if (fetchError != '') {
    return gotFetchError(fetchError)
  }
  // We do this just like any other fetch to ensure the variables get loaded from
  // the sippy API before any consumers use those variables or the expandEnvironment
  // function that depends on them.I think
  if (!isLoaded) {
    return (
      <CompReadyProgress
        apiLink={'Loading /variant info ...'}
        cancelFunc={cancelFetch}
      />
    )
  }
  return (
    <CompReadyVarsContext.Provider
      value={{
        excludeNetworksList,
        excludeCloudsList,
        excludeArchesList,
        excludeUpgradesList,
        excludeVariantsList,
        expandEnvironment,
        baseRelease,
        setBaseReleaseWithDates,
        sampleRelease,
        setSampleReleaseWithDates,
        baseStartTime,
        setBaseStartTime,
        baseEndTime,
        setBaseEndTime,
        sampleStartTime,
        setSampleStartTime,
        sampleEndTime,
        setSampleEndTime,
        groupByCheckedItems,
        setGroupByCheckedItems,
        excludeUpgradesCheckedItems,
        setExcludeUpgradesCheckedItems,
        excludeVariantsCheckedItems,
        setExcludeVariantsCheckedItems,
        excludeCloudsCheckedItems,
        setExcludeCloudsCheckedItems,
        excludeArchesCheckedItems,
        setExcludeArchesCheckedItems,
        excludeNetworksCheckedItems,
        setExcludeNetworksCheckedItems,
        confidence,
        setConfidence,
        pity,
        setPity,
        minFail,
        setMinFail,
        ignoreMissing,
        setIgnoreMissing,
        ignoreDisruption,
        setIgnoreDisruption,
        component,
        setComponentParam,
        capability,
        setCapabilityParam,
        environment,
        setEnvironmentParam,
        handleGenerateReport,
      }}
    >
      {children}
    </CompReadyVarsContext.Provider>
  )
}

CompReadyVarsProvider.propTypes = {
  children: PropTypes.node.isRequired,
}
