import {
  ArrayParam,
  BooleanParam,
  NumberParam,
  ObjectParam,
  SafeStringParam,
  StringParam,
  useQueryParam,
} from 'use-query-params'
import {
  convertParamToVariantItems,
  convertVariantItemsToParam,
  dateEndFormat,
  dateFormat,
  formatLongDate,
  getComponentReadinessViewsUrl,
  getJobVariantsUrl,
  gotFetchError,
} from './CompReadyUtils'
import { ReleasesContext } from '../App'
import { safeEncodeURIComponent } from '../helpers'
import CompReadyProgress from './CompReadyProgress'
import PropTypes from 'prop-types'
import React, { createContext, useContext, useEffect, useState } from 'react'
export const CompReadyVarsContext = createContext()

export const CompReadyVarsProvider = ({ children }) => {
  const [allJobVariants, setAllJobVariants] = useState([])
  const [views, setViews] = useState([])
  const [view, setView] = useQueryParam('view', StringParam)
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
  const initialSampleStartTime = new Date(now.getTime() - 7 * days)
  const initialSampleEndTime = new Date(now.getTime())

  // Base is 28 days from the default base release's GA date
  // Match what the metrics uses in the api.
  const initialBaseStartTime =
    getReleaseDate(defaultBaseRelease).getTime() - 27 * days
  const initialBaseEndTime =
    getReleaseDate(defaultBaseRelease).getTime() + 1 * days - 1 * seconds

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
    columnGroupByCheckedItemsParam = ['Platform', 'Architecture', 'Network'],
    setColumnGroupByCheckedItemsParam,
  ] = useQueryParam('columnGroupBy', ArrayParam)

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

  // Create the variables to be used for api calls; these are initialized to the
  // value of the variables that got their values from the URL.
  const [columnGroupByCheckedItems, setColumnGroupByCheckedItems] =
    React.useState(columnGroupByCheckedItemsParam)

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

  const [baseRelease, setBaseRelease] = React.useState(baseReleaseParam)

  const [sampleRelease, setSampleRelease] = React.useState(sampleReleaseParam)

  const [baseStartTime, setBaseStartTime] = React.useState(baseStartTimeParam)

  const [baseEndTime, setBaseEndTime] = React.useState(baseEndTimeParam)

  const [sampleStartTime, setSampleStartTime] =
    React.useState(sampleStartTimeParam)
  const [sampleEndTime, setSampleEndTime] = React.useState(sampleEndTimeParam)

  const [samplePROrgParam = '', setSamplePROrgParam] = useQueryParam(
    'samplePROrg',
    StringParam
  )
  const [samplePROrg, setSamplePROrg] = React.useState(samplePROrgParam)
  const [samplePRRepoParam = '', setSamplePRRepoParam] = useQueryParam(
    'samplePRRepo',
    StringParam
  )
  const [samplePRRepo, setSamplePRRepo] = React.useState(samplePRRepoParam)
  const [samplePRNumberParam = '', setSamplePRNumberParam] = useQueryParam(
    'samplePRNumber',
    StringParam
  )
  const [samplePRNumber, setSamplePRNumber] =
    React.useState(samplePRNumberParam)

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

  /********************************************************
   * All things related to parameter selection for variants
   ******************************************************** */

  /** URL parameters for variants **/
  // The variants that have been selected for inclusion in the basis and sample (unless they are in the cross-compare list)
  const [
    includeVariantsCheckedItemsParam = [
      'Architecture:amd64',
      'FeatureSet:default',
      'Installer:ipi',
      'Installer:upi',
      'Owner:eng',
      'Platform:aws',
      'Platform:azure',
      'Platform:gcp',
      'Platform:metal',
      'Platform:vsphere',
      'Topology:ha',
    ],
    setIncludeVariantsCheckedItemsParam,
  ] = useQueryParam('includeVariant', ArrayParam)
  // The list of variant groups (e.g. "Architecture") that have been selected for cross-variant comparison
  const [variantCrossCompareParam = [], setVariantCrossCompareParam] =
    useQueryParam('variantCrossCompare', ArrayParam)
  // The list of individual variants (e.g. "Architecture:arm64") that are checked for cross-variant comparison
  const [
    compareVariantsCheckedItemsParam = [],
    setCompareVariantsCheckedItemsParam,
  ] = useQueryParam('compareVariant', ArrayParam)

  /** some state variables and related handlers for managing the display of variants in the UI **/
  // The grouped variants that have been selected for inclusion in the basis and sample (unless they are in the cross-compare list)
  const [includeVariantsCheckedItems, setIncludeVariantsCheckedItems] =
    useState(convertParamToVariantItems(includeVariantsCheckedItemsParam))
  const replaceIncludeVariantsCheckedItems = (variant, checkedItems) => {
    includeVariantsCheckedItems[variant] = checkedItems
    // this state stuff seems redundant but when omitted, params don't update reliably
    setIncludeVariantsCheckedItems(includeVariantsCheckedItems)
  }
  // The grouped variants that have been selected for cross-comparison in the sample
  const [compareVariantsCheckedItems, setCompareVariantsCheckedItems] =
    useState(convertParamToVariantItems(compareVariantsCheckedItemsParam))
  const replaceCompareVariantsCheckedItems = (variant, checkedItems) => {
    compareVariantsCheckedItems[variant] = checkedItems
    // this state stuff seems redundant but when omitted, params don't update reliably
    setCompareVariantsCheckedItems(compareVariantsCheckedItems)
  }
  // This is the list of variant groups (e.g. "Architecture") that have been selected for cross-variant comparison
  const [variantCrossCompare, setVariantCrossCompare] = useState(
    variantCrossCompareParam
  )
  // This is run when the user (un)selects a variant group (e.g. "Platform") for cross-variant comparison.
  const updateVariantCrossCompare = (variantGroupName, isCompareMode) => {
    setVariantCrossCompare(
      isCompareMode
        ? [...variantCrossCompare, variantGroupName]
        : variantCrossCompare.filter((name) => name !== variantGroupName)
    )
  }

  /********************************************************
   * All things related to the "Advanced" section
   ******************************************************** */

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

  /******************************************************************************
   * Parameters that are used to refine the query as the user drills down into CR
   ****************************************************************************** */

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

  /******************************************************************************
   * Generating the report parameters:
   ****************************************************************************** */

  // dbGroupByVariants defines what variants are used for GroupBy in DB query
  const dbGroupByVariants = [
    'Platform',
    'Architecture',
    'Network',
    'Topology',
    'FeatureSet',
    'Upgrade',
    'Suite',
    'Installer',
  ]

  // This runs when someone pushes the "Generate Report" button.
  // It sets all parameters based on current state; this causes the URL to be updated and page to load with new params.
  const handleGenerateReport = (event) => {
    event.preventDefault()

    // If the generate report button was pressed, views are out of the question and we're now
    // fully qualifying all params:
    setView('')

    setBaseReleaseParam(baseRelease)
    setBaseStartTimeParam(formatLongDate(baseStartTime, dateFormat))
    setBaseEndTimeParam(formatLongDate(baseEndTime, dateEndFormat))
    setSampleReleaseParam(sampleRelease)
    setSampleStartTimeParam(formatLongDate(sampleStartTime, dateFormat))
    setSampleEndTimeParam(formatLongDate(sampleEndTime, dateEndFormat))

    setColumnGroupByCheckedItemsParam(columnGroupByCheckedItems)
    setIncludeVariantsCheckedItemsParam(
      convertVariantItemsToParam(includeVariantsCheckedItems)
    )
    setCompareVariantsCheckedItemsParam(
      convertVariantItemsToParam(compareVariantsCheckedItems)
    )
    setVariantCrossCompareParam(variantCrossCompare)
    setConfidenceParam(confidence)
    setSamplePROrgParam(samplePROrg)
    setSamplePRRepoParam(samplePRRepo)
    setSamplePRNumberParam(samplePRNumber)
    setPityParam(pity)
    setMinFailParam(minFail)
    setIgnoreDisruptionParam(ignoreDisruption)
    setIgnoreMissingParam(ignoreMissing)
    setComponentParam(component)
    setEnvironmentParam(environment)
    setCapabilityParam(capability)
  }

  const clearAllQueryParams = () => {
    // Because all our Param properties have state linked to the URL shown, if we select a view,
    // we have to clear them all out otherwise we see the view, but the URL continues to
    // show our last selected query params.
    setBaseStartTimeParam(undefined)
    setBaseEndTimeParam(undefined)
    setBaseReleaseParam(undefined)

    setSampleStartTimeParam(undefined)
    setSampleEndTimeParam(undefined)
    setSampleReleaseParam(undefined)

    setColumnGroupByCheckedItemsParam(undefined)
    setIncludeVariantsCheckedItemsParam(undefined)
    setVariantCrossCompareParam(undefined)
    setCompareVariantsCheckedItemsParam(undefined)

    setConfidenceParam(undefined)
    setPityParam(undefined)
    setMinFailParam(undefined)
    setIgnoreDisruptionParam(undefined)
    setIgnoreMissingParam(undefined)

    setSamplePROrgParam(undefined)
    setSamplePRRepoParam(undefined)
    setSamplePRNumberParam(undefined)
  }

  // syncView updates all vars and thus their respective inputs to match a server side view that was
  // just selected by the user.
  const syncView = (view) => {
    clearAllQueryParams()
    setBaseRelease(view.base_release.release)
    setBaseStartTime(formatLongDate(view.base_release.start, dateFormat))
    setBaseEndTime(formatLongDate(view.base_release.end, dateFormat))
    setSampleRelease(view.sample_release.release)
    setSampleStartTime(formatLongDate(view.sample_release.start, dateFormat))
    setSampleEndTime(formatLongDate(view.sample_release.end, dateFormat))

    console.log('columnGroupBy: ' + columnGroupByCheckedItems[0])

    // Build array of columns to group by given the view:
    setColumnGroupByCheckedItems(
      Object.keys(view.variant_options.column_group_by)
    )

    if (view.variant_options.hasOwnProperty('include_variants')) {
      setIncludeVariantsCheckedItems(view.variant_options.include_variants)
    }
    if (view.variant_options.hasOwnProperty('variant_cross_compare')) {
      setVariantCrossCompare(view.variant_options.variant_cross_compare)
    }
    if (view.variant_options.hasOwnProperty('compare_variants')) {
      setCompareVariantsCheckedItems(view.variant_options.compare_variants)
    }

    if (view.advanced_options.hasOwnProperty('confidence')) {
      setConfidence(view.advanced_options.confidence)
    }
    if (view.advanced_options.hasOwnProperty('pity_factor')) {
      setPity(view.advanced_options.pity_factor)
    }
    if (view.advanced_options.hasOwnProperty('minimum_failure')) {
      setMinFail(view.advanced_options.minimum_failure)
    }
    if (view.advanced_options.hasOwnProperty('ignore_disruption')) {
      setIgnoreDisruption(view.advanced_options.ignore_disruption)
    }
    if (view.advanced_options.hasOwnProperty('ignore_missing')) {
      setIgnoreMissing(view.advanced_options.ignore_missing)
    }
  }

  useEffect(() => {
    const jobVariantsAPIURL = getJobVariantsUrl()
    const viewsAPIURL = getComponentReadinessViewsUrl()
    Promise.all([fetch(jobVariantsAPIURL), fetch(viewsAPIURL)])
      .then(([variantsResp, viewsResp]) => {
        if (variantsResp.code < 200 || variantsResp.code >= 300) {
          const errorMessage = variantsResp.message
            ? `${variantsResp.message}`
            : 'No error message'
          throw new Error(
            `Return code = ${variantsResp.code} (${errorMessage})`
          )
        }
        if (viewsResp.code < 200 || viewsResp.code >= 300) {
          const errorMessage = viewsResp.message
            ? `${viewsResp.message}`
            : 'No error message'
          throw new Error(`Return code = ${viewsResp.code} (${errorMessage})`)
        }
        return Promise.all([variantsResp.json(), viewsResp.json()])
      })
      .then(([variants, views]) => {
        setAllJobVariants(variants.variants)
        setViews(views)
        if (views.length > 0) {
          // Default view should be the first one in the list matching our defaultSampleRelease
          views.forEach((view) => {
            if (view.sample_release.release === defaultSampleRelease) {
              setView(view.name)
              syncView(view)
            }
          })
        }
        setIsLoaded(true)
      })
      .catch((error) => {
        setFetchError(`API call failed: ${error}`)
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
      Object.entries(allJobVariants).forEach(([variantName, variantValues]) => {
        if (variantValues.includes(item)) {
          params[variantName] = item
        }
      })
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
        allJobVariants,
        views,
        view,
        setView,
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
        samplePROrg,
        setSamplePROrg,
        samplePRRepo,
        setSamplePRRepo,
        samplePRNumber,
        setSamplePRNumber,
        columnGroupByCheckedItems,
        setColumnGroupByCheckedItems,
        dbGroupByVariants,
        includeVariantsCheckedItems,
        replaceIncludeVariantsCheckedItems,
        compareVariantsCheckedItems,
        replaceCompareVariantsCheckedItems,
        variantCrossCompare,
        updateVariantCrossCompare,
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
        syncView,
      }}
    >
      {children}
    </CompReadyVarsContext.Provider>
  )
}

CompReadyVarsProvider.propTypes = {
  children: PropTypes.node.isRequired,
}
