import {
  ArrayParam,
  NumberParam,
  StringParam,
  useQueryParams,
} from 'use-query-params'
import {
  convertParamToVariantItems,
  convertVariantItemsToParam,
  dateEndFormat,
  dateFormat,
  formatLongDate,
  getComponentReadinessViewsAPIUrl,
  getJobVariantsAPIUrl,
  gotFetchError,
} from './CompReadyUtils'
import { ReleasesContext } from '../App'
import { safeEncodeURIComponent, SafeStringParam } from '../helpers'
import CompReadyProgress from './CompReadyProgress'
import PropTypes from 'prop-types'
import React, { createContext, useContext, useEffect, useState } from 'react'

export const CompReadyVarsContext = createContext({})

// Use of booleans in URL params does not seem to parse properly as a BooleanParam.
// Use this custom param parser instead.
const CustomBooleanParam = {
  encode: (value) => {
    if (value === null || value === undefined) return undefined
    return String(value)
  },
  decode: (value) => {
    if (value === 'true' || value === '1' || value === '') return true
    if (value === 'false' || value === '0') return false
    return undefined
  },
}

const defaultIncludeVariants = [
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
  'CGroupMode:v2',
  'ContainerRuntime:runc',
]

// with ReleaseContext, use the list of GA releases and their dates to determine the default base and sample releases.
function gaReleaseInfo(releases) {
  const gaReleases = Object.keys(releases.ga_dates)
  gaReleases.sort(
    (a, b) => new Date(releases.ga_dates[b]) - new Date(releases.ga_dates[a])
  )
  let defaultBaseRelease = gaReleases[0]

  // default sample is the first release that has previous_release = defaultBaseRelease
  let defaultSampleRelease = null
  for (let name of releases.releases) {
    if (releases.release_attrs[name]?.previous_release === defaultBaseRelease) {
      defaultSampleRelease = name
    }
  }

  const getReleaseDate = (release) => {
    if (releases.ga_dates && releases.ga_dates[release]) {
      return new Date(releases.ga_dates[release])
    }

    return new Date()
  }
  return { defaultBaseRelease, defaultSampleRelease, getReleaseDate }
}

export const CompReadyVarsProvider = ({ children }) => {
  // some state for the actual process of loading data from the API
  const [allJobVariants, setAllJobVariants] = useState([])
  const [views, setViews] = useState([])
  const [isLoaded, setIsLoaded] = useState(false)
  const [fetchError, setFetchError] = useState('')

  // read all the parameters from the URL that we care about for component readiness pages.
  const [params, setParams] = useQueryParams({
    view: StringParam,
    baseRelease: StringParam,
    baseStartTime: StringParam,
    baseEndTime: StringParam,
    sampleRelease: StringParam,
    sampleStartTime: StringParam,
    sampleEndTime: StringParam,
    confidence: NumberParam,
    pity: NumberParam,
    minFail: NumberParam,
    passRateNewTests: NumberParam,
    passRateAllTests: NumberParam,
    ignoreMissing: CustomBooleanParam,
    ignoreDisruption: CustomBooleanParam,
    includeMultiReleaseAnalysis: CustomBooleanParam,
    flakeAsFailure: CustomBooleanParam,
    component: SafeStringParam,
    environment: StringParam,
    capability: StringParam,
    testId: StringParam,
    testName: StringParam,
    testBasisRelease: StringParam,
    samplePROrg: StringParam,
    samplePRRepo: StringParam,
    samplePRNumber: StringParam,
    samplePayloadTags: ArrayParam,
    dbGroupBy: StringParam, // This is comma-separated in the URL, e.g. Platform,Architecture,...
    columnGroupBy: StringParam, // This is comma-separated in the URL, e.g. Platform,Network,Architecture
    includeVariant: ArrayParam, // variants selected for inclusion in the basis and sample (unless cross-compared)
    variantCrossCompare: ArrayParam, // variant groups (e.g. "Architecture") selected for cross-variant comparison
    compareVariant: ArrayParam, // individual variants (e.g. "Architecture:arm64") checked for cross-variant comparison
  })

  // Find the most recent GA releases
  const { defaultBaseRelease, defaultSampleRelease, getReleaseDate } =
    gaReleaseInfo(useContext(ReleasesContext))
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

  // Create the variables to be used for UI URLs or api calls; these are initialized to the
  // value of the variables that got their values from the URL.
  const [columnGroupByCheckedItems, setColumnGroupByCheckedItems] =
    React.useState([])
  const [dbGroupByVariants, setDbGroupByVariants] = React.useState([])
  const [baseRelease, setBaseRelease] = React.useState('')
  const [sampleRelease, setSampleRelease] = React.useState('')
  const [baseStartTime, setBaseStartTime] = React.useState('')
  const [baseEndTime, setBaseEndTime] = React.useState('')
  const [sampleStartTime, setSampleStartTime] = React.useState('')
  const [sampleEndTime, setSampleEndTime] = React.useState('')
  const [samplePROrg, setSamplePROrg] = React.useState('')
  const [samplePRRepo, setSamplePRRepo] = React.useState('')
  const [samplePRNumber, setSamplePRNumber] = React.useState('')
  const [samplePayloadTags, setSamplePayloadTags] = React.useState([])

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

  /** some state variables and related handlers for managing the display of variants in the UI **/
  // The grouped variants that have been selected for inclusion in the basis and sample (unless they are in the cross-compare list)
  const [includeVariantsCheckedItems, setIncludeVariantsCheckedItems] =
    useState({})
  const replaceIncludeVariantsCheckedItems = (variant, checkedItems) => {
    includeVariantsCheckedItems[variant] = checkedItems
    setIncludeVariantsCheckedItems(includeVariantsCheckedItems)
  }
  // The list of individual variants (e.g. "Architecture:arm64") that are checked for cross-variant comparison
  const [compareVariantsCheckedItems, setCompareVariantsCheckedItems] =
    useState([])
  const replaceCompareVariantsCheckedItems = (variant, checkedItems) => {
    compareVariantsCheckedItems[variant] = checkedItems
    setCompareVariantsCheckedItems(compareVariantsCheckedItems)
  }
  // The list of variant groups (e.g. "Architecture") that have been selected for cross-variant comparison
  const [variantCrossCompare, setVariantCrossCompare] = useState([])
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

  const [confidence, setConfidence] = React.useState(0)
  const [pity, setPity] = React.useState(0)
  const [minFail, setMinFail] = React.useState(0)
  const [passRateNewTests, setPassRateNewTests] = React.useState(0)
  const [passRateAllTests, setPassRateAllTests] = React.useState(0)

  const [ignoreMissing, setIgnoreMissing] = React.useState(false)
  const [ignoreDisruption, setIgnoreDisruption] = React.useState(false)
  const [flakeAsFailure, setFlakeAsFailure] = React.useState(false)
  const [includeMultiReleaseAnalysis, setIncludeMultiReleaseAnalysis] =
    React.useState(false)
  /******************************************************************************
   * Parameters that are used to refine the query as the user drills down into CR
   ****************************************************************************** */

  const [component, setComponent] = React.useState(undefined)
  const [environment, setEnvironment] = React.useState(undefined)
  const [capability, setCapability] = React.useState(undefined)
  const [testId, setTestId] = React.useState(undefined)
  const [testName, setTestName] = React.useState(undefined)
  const [testBasisRelease, setTestBasisRelease] = React.useState(undefined)

  // at initialization and whenever URL params change, update the state variables to match the URL and view.
  useEffect(() => {
    // Initialize pity
    setPity(params.pity || 5)

    // Initialize columnGroupByCheckedItems
    setColumnGroupByCheckedItems(
      params.columnGroupBy
        ? params.columnGroupBy.split(',')
        : ['Platform', 'Architecture', 'Network']
    )

    // Initialize dbGroupByVariants
    setDbGroupByVariants(
      params.dbGroupBy
        ? params.dbGroupBy.split(',')
        : [
            'Platform',
            'Architecture',
            'Network',
            'Topology',
            'FeatureSet',
            'Upgrade',
            'Suite',
            'Installer',
          ]
    )

    // Initialize baseRelease
    setBaseRelease(params.baseRelease || defaultBaseRelease)

    // Initialize sampleRelease
    setSampleRelease(params.sampleRelease || defaultSampleRelease)

    // Initialize baseStartTime
    setBaseStartTime(
      params.baseStartTime || formatLongDate(initialBaseStartTime, dateFormat)
    )

    // Initialize baseEndTime
    setBaseEndTime(
      params.baseEndTime || formatLongDate(initialBaseEndTime, dateEndFormat)
    )

    // Initialize sampleStartTime
    setSampleStartTime(
      params.sampleStartTime ||
        formatLongDate(initialSampleStartTime, dateFormat)
    )

    // Initialize sampleEndTime
    setSampleEndTime(
      params.sampleEndTime ||
        formatLongDate(initialSampleEndTime, dateEndFormat)
    )

    // Initialize samplePR related fields
    setSamplePROrg(params.samplePROrg || '')
    setSamplePRRepo(params.samplePRRepo || '')
    setSamplePRNumber(params.samplePRNumber || '')
    setSamplePayloadTags(params.samplePayloadTags || [])

    // Initialize includeVariantsCheckedItems
    setIncludeVariantsCheckedItems(
      convertParamToVariantItems(
        params.includeVariant || defaultIncludeVariants
      )
    )

    // Initialize compareVariantsCheckedItems
    setCompareVariantsCheckedItems(
      convertParamToVariantItems(params.compareVariant || [])
    )

    // Initialize variantCrossCompare
    setVariantCrossCompare(params.variantCrossCompare || [])

    // Initialize advanced section fields
    setConfidence(params.confidence || 95)
    setMinFail(params.minFail || 3)
    setPassRateNewTests(params.passRateNewTests || 0)
    setPassRateAllTests(params.passRateAllTests || 0)

    // Initialize boolean fields
    setIgnoreMissing(params.ignoreMissing || false)
    setIgnoreDisruption(params.ignoreDisruption || true)
    setFlakeAsFailure(params.flakeAsFailure || false)
    setIncludeMultiReleaseAnalysis(params.includeMultiReleaseAnalysis || false)

    // Initialize drill-down parameters
    setComponent(params.component)
    setEnvironment(params.environment)
    setCapability(params.capability)
    setTestId(params.testId)
    setTestName(params.testName)
    setTestBasisRelease(params.testBasisRelease)

    updateVarsFromView(params.view, views)
  }, [params])

  /******************************************************************************
   * Generating the report parameters:
   ****************************************************************************** */

  // This runs when someone pushes the "Generate Report" button.
  // It sets all parameters based on current state; this causes the URL to be updated and page to load with new params.
  const handleGenerateReport = (event, callback) => {
    if (event && event.preventDefault) {
      event.preventDefault()
    }

    // "Generate report" button was pressed, unset "view" while specifying all other params
    setParams({
      view: undefined,
      baseRelease,
      baseStartTime: formatLongDate(baseStartTime, dateFormat),
      baseEndTime: formatLongDate(baseEndTime, dateEndFormat),
      sampleRelease,
      sampleStartTime: formatLongDate(sampleStartTime, dateFormat),
      sampleEndTime: formatLongDate(sampleEndTime, dateEndFormat),
      confidence,
      pity,
      minFail,
      passRateNewTests,
      passRateAllTests,
      ignoreMissing,
      ignoreDisruption,
      includeMultiReleaseAnalysis,
      flakeAsFailure,
      component,
      environment,
      capability,
      testId,
      testName,
      testBasisRelease,
      samplePROrg,
      samplePRRepo,
      samplePRNumber,
      samplePayloadTags,
      dbGroupBy: dbGroupByVariants.join(','),
      columnGroupBy: columnGroupByCheckedItems.join(','),
      includeVariant: convertVariantItemsToParam(includeVariantsCheckedItems),
      variantCrossCompare: variantCrossCompare,
      compareVariant: convertVariantItemsToParam(compareVariantsCheckedItems),
    })
  }

  // syncView updates all vars and thus their respective inputs to match a server side view that was
  // just selected by the user.
  const syncView = (view) => {
    let nonView = {} // wipe out current URL params except the view
    for (const key in params) {
      nonView[key] = undefined
    }
    setParams({ ...nonView, view: view.name })
    updateVarsFromView(view.name, views)
  }

  const updateVarsFromView = (viewName, loadedViews = []) => {
    if (params.view === undefined || loadedViews.length === 0) return
    // A view query param was requested and we have our views list; sync the controls to match:
    let view = loadedViews.find((v) => v.name === viewName)
    if (!view) return // TODO: alert when view not found in loaded views?

    setBaseRelease(view.base_release.release)
    setBaseStartTime(formatLongDate(view.base_release.start, dateFormat))
    setBaseEndTime(formatLongDate(view.base_release.end, dateFormat))
    setSampleRelease(view.sample_release.release)
    setSampleStartTime(formatLongDate(view.sample_release.start, dateFormat))
    setSampleEndTime(formatLongDate(view.sample_release.end, dateFormat))

    // Build array of columns to group by given the view:
    setColumnGroupByCheckedItems(
      Object.keys(view.variant_options.column_group_by)
    )
    setDbGroupByVariants(Object.keys(view.variant_options.db_group_by))

    if (view.variant_options.hasOwnProperty('include_variants'))
      setIncludeVariantsCheckedItems(view.variant_options.include_variants)
    if (view.variant_options.hasOwnProperty('variant_cross_compare'))
      setVariantCrossCompare(view.variant_options.variant_cross_compare)
    if (view.variant_options.hasOwnProperty('compare_variants'))
      setCompareVariantsCheckedItems(view.variant_options.compare_variants)
    if (view.advanced_options.hasOwnProperty('confidence'))
      setConfidence(view.advanced_options.confidence)
    if (view.advanced_options.hasOwnProperty('pity_factor'))
      setPity(view.advanced_options.pity_factor)
    if (view.advanced_options.hasOwnProperty('minimum_failure'))
      setMinFail(view.advanced_options.minimum_failure)
    if (view.advanced_options.hasOwnProperty('pass_rate_required_new_tests'))
      setPassRateNewTests(view.advanced_options.pass_rate_required_new_tests)
    if (view.advanced_options.hasOwnProperty('pass_rate_required_all_tests'))
      setPassRateAllTests(view.advanced_options.pass_rate_required_all_tests)
    if (view.advanced_options.hasOwnProperty('ignore_disruption'))
      setIgnoreDisruption(view.advanced_options.ignore_disruption)
    if (view.advanced_options.hasOwnProperty('ignore_missing'))
      setIgnoreMissing(view.advanced_options.ignore_missing)
    if (view.advanced_options.hasOwnProperty('flake_as_failure'))
      setFlakeAsFailure(view.advanced_options.flake_as_failure)
    if (view.advanced_options.hasOwnProperty('include_multi_release_analysis'))
      setIncludeMultiReleaseAnalysis(
        view.advanced_options.include_multi_release_analysis
      )
  }

  useEffect(() => {
    const jobVariantsAPIURL = getJobVariantsAPIUrl()
    const viewsAPIURL = getComponentReadinessViewsAPIUrl()
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
          // If no view was requested and we were not given fully qualified params,
          // select the default view: (first in the list matching our default sample release)
          if (shouldLoadDefaultView()) {
            for (let v of views) {
              var foundView = false
              if (v.sample_release.release === defaultSampleRelease) {
                foundView = true
                syncView(v)
                break
              }
            }
            // Catch case where the views file has no entry for the current default sample release:
            if (!foundView) {
              syncView(views[0])
            }
          } else updateVarsFromView(params.view, views)
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

  const shouldLoadDefaultView = () => {
    // Attempt to decide if we should pre-select the default view, or if we were given params:
    return params.view === undefined && params.baseRelease === undefined
  }

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
      const variants = item.split(':')
      if (variants.length === 2) {
        params[variants[0]] = variants[1]
      } else {
        // handle legacy links where env didn't specify variant name (TRT-2154)
        Object.entries(allJobVariants).forEach(
          ([variantName, variantValues]) => {
            // the new env format started after platform 'none' was added.
            // we only need to support 'none' in the legacy format for Upgrade=none
            if (variantValues.includes(item))
              if (variantName === 'Upgrade' || item !== 'none')
                params[variantName] = item
          }
        )
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
    return <CompReadyProgress apiLink={'Loading /variant info ...'} />
  }
  return (
    <CompReadyVarsContext.Provider
      value={{
        urlParams: params,
        view: params.view,
        views,
        allJobVariants,
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
        samplePayloadTags,
        setSamplePayloadTags,
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
        passRateNewTests,
        setPassRateNewTests,
        passRateAllTests,
        setPassRateAllTests,
        ignoreMissing,
        setIgnoreMissing,
        ignoreDisruption,
        setIgnoreDisruption,
        flakeAsFailure,
        setFlakeAsFailure,
        includeMultiReleaseAnalysis,
        setIncludeMultiReleaseAnalysis,
        component,
        capability,
        environment,
        testId,
        testName,
        testBasisRelease,
        handleGenerateReport,
        syncView,
        isLoaded,
      }}
    >
      {children}
    </CompReadyVarsContext.Provider>
  )
}

CompReadyVarsProvider.propTypes = {
  children: PropTypes.node.isRequired,
}
