import { AccessibilityModeContext } from '../components/AccessibilityModeProvider'
import { alpha, InputBase, Typography } from '@mui/material'
import { formatInTimeZone } from 'date-fns-tz'
import { styled } from '@mui/styles'
import Alert from '@mui/material/Alert'
import blue from './blue.svg'
import blue_missing_data from './none-blue.svg'
import fix_failed from './fix_failed.svg'
import fix_failed_accessible from './fix_failed_accessible.svg'
import fixed_waiting from './fixed_waiting.svg'
import fixed_waiting_accessible from './fixed_waiting_accessible.svg'
import green from './green.svg'
import green_half_data from './half.svg'
import green_missing_data from './none.svg'
import half_blue from './half-blue.svg'
import heart from './improved.svg'
import orange from './orange.svg'
import orange_3d from './extreme-orange.svg'
import orange_3d_triaged from './extreme-orange-triaged.svg'
import orange_triaged from './orange-triaged.svg'
import React, { useContext } from 'react'
import red from './regressed.svg'
import red_3d from './extreme.svg'
import red_3d_triaged from './extreme-triaged.svg'
import red_triaged from './regressed-triaged.svg'

// Set to true for debug mode
export const debugMode = false

// Make the HH:mm:ss as zeros to be more conducive to caching query caching.
export const dateFormat = 'yyyy-MM-dd 00:00:00'
export const dateEndFormat = 'yyyy-MM-dd 23:59:59'

// This is the table we use when the first page is initially rendered.
export const initialPageTable = {
  rows: [
    {
      component: 'None',
      columns: [
        {
          empty: 'None',
          status: 3, // Let's start with success
          regressed_tests: [],
          variants: [],
        },
      ],
    },
  ],
}
export const noDataTable = {
  rows: [
    {
      component: 'No Data found',
      columns: [
        {
          empty: 'None',
          status: 3, // Let's start with success
          variants: [],
        },
      ],
    },
  ],
}
export const cancelledDataTable = {
  rows: [
    {
      component: 'Cancelled',
      columns: [
        {
          empty: 'None',
          status: 3, // Let's start with success
          variants: [],
        },
      ],
    },
  ],
}
export const jiraUrlPrefixDeprecated = 'https://issues.redhat.com/browse/'
export const jiraUrlPrefix = 'https://redhat.atlassian.net/browse/'

// Make one place to create the Component Readiness api call
export function getAPIUrl(endpoint) {
  return `${process.env.REACT_APP_API_URL}/api/${endpoint}`
}

export function getCRMainAPIUrl() {
  return getAPIUrl('component_readiness')
}

export function getJobVariantsAPIUrl() {
  return getAPIUrl('job_variants')
}

export function getComponentReadinessViewsAPIUrl() {
  return getAPIUrl('component_readiness/views')
}

export function getTriagesAPIUrl(id = null) {
  return getAPIUrl(
    id ? `component_readiness/triages/${id}` : 'component_readiness/triages'
  )
}

export function getBugsAPIUrl() {
  return getAPIUrl(`component_readiness/bugs`)
}

export function getRegressionAPIUrl(id) {
  return getAPIUrl(`component_readiness/regressions/${id}`)
}

export function getArtifactQueryAPIUrl() {
  return getAPIUrl('jobs/artifacts')
}

export const gotoCompReadyMain = () => {
  window.location.href = '/sippy-ng/component_readiness/main'
}

// When we get a fetch error, this will print a standard message.
export function gotFetchError(fetchError) {
  return (
    <Alert severity="error">
      <h2>Failed to load component readiness data</h2>
      <h3>
        {fetchError.split('\n').map((item) => (
          <>
            <hr />
            {item}
          </>
        ))}
      </h3>
      <hr />
      <h3>Check, and possibly fix api server, then click below to retry</h3>
      <button onClick={gotoCompReadyMain}>Retry</button>
    </Alert>
  )
}

// getStatusAndIcon returns a status string and icon to display to denote a visual and textual
// meaning of a 'status' value.  We optionally allow a grayscale mode for the red colors.
export function getStatusAndIcon(
  status,
  grayFactor = 0,
  accessibilityMode = false
) {
  let icon = ''

  let statusStr = status + ': '

  if (status >= 300) {
    statusStr =
      statusStr + 'SignificantImprovement detected (improved sample rate)'
    icon = (
      <img
        alt="SignificantImprovement"
        src={heart}
        width="24px"
        height="24px"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status === 200) {
    statusStr =
      statusStr + 'Missing Basis And Sample (basis and sample data missing)'
    let src = accessibilityMode ? blue_missing_data : green_missing_data
    icon = (
      <img
        src={src}
        alt="MissingBasisAndSample"
        width="24px"
        height="24px"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status === 100) {
    statusStr = statusStr + 'Missing Basis (basis data missing)'
    let src = accessibilityMode ? half_blue : green_half_data
    icon = (
      <img
        src={src}
        alt="MissingBasis"
        width="24px"
        height="24px"
        style={{
          filter: `grayscale(${grayFactor}%)`,
        }}
      />
    )
  } else if (status === 0) {
    statusStr = statusStr + 'NoSignificantDifference detected'
    let src = accessibilityMode ? blue : green
    icon = (
      <img
        src={src}
        width="24px"
        height="24px"
        alt="NotSignificant"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status === -100) {
    statusStr = statusStr + 'Missing Sample (sample data missing)'
    let src = accessibilityMode ? half_blue : green_half_data
    icon = (
      <img
        src={src}
        alt="MissingSample"
        width="24px"
        height="24px"
        style={{
          transform: `rotate(180deg)`,
          filter: `grayscale(${grayFactor}%)`,
        }}
      />
    )
  } else if (status === -150) {
    statusStr = statusStr + 'Fixed (hopefully) regression detected'
    let src = accessibilityMode ? fixed_waiting_accessible : fixed_waiting
    icon = <img width="24px" height="24px" src={src} alt="Fixed regression" />
  } else if (status === -200) {
    statusStr = statusStr + 'SignificantTriagedRegression detected'
    let src = accessibilityMode ? orange_triaged : red_triaged
    icon = (
      <img
        width="24px"
        height="24px"
        src={src}
        alt="SignificantTriagedRegression"
      />
    )
  } else if (status === -300) {
    statusStr =
      statusStr + 'ExtremeTriagedRegression detected ( >15% pass rate change)'
    let src = accessibilityMode ? orange_3d_triaged : red_3d_triaged
    icon = (
      <img
        width="24px"
        height="24px"
        src={src}
        alt="ExtremeTriagedRegression >15%"
      />
    )
  } else if (status === -400) {
    statusStr = statusStr + 'SignificantRegression detected'
    let src = accessibilityMode ? orange : red
    icon = (
      <img width="24px" height="24px" src={src} alt="SignificantRegression" />
    )
  } else if (status === -500) {
    statusStr =
      statusStr + 'ExtremeRegression detected ( >15% pass rate change)'
    let src = accessibilityMode ? orange_3d : red_3d
    icon = (
      <img width="24px" height="24px" src={src} alt="ExtremeRegression >15%" />
    )
  } else if (status === -1000) {
    statusStr = statusStr + 'Failed fix detected'
    let src = accessibilityMode ? fix_failed_accessible : fix_failed
    icon = <img width="24px" height="24px" src={src} alt="Fixed regression" />
  }

  return [statusStr, icon]
}

export function StatusLegend() {
  const { accessibilityModeOn } = useContext(AccessibilityModeContext)
  const a11y = accessibilityModeOn
  const items = [
    { src: a11y ? blue : green, label: 'No Significant Difference' },
    { src: a11y ? half_blue : green_half_data, label: 'Missing Basis' },
    {
      src: a11y ? blue_missing_data : green_missing_data,
      label: 'Missing Basis & Sample',
    },
    {
      src: a11y ? half_blue : green_half_data,
      label: 'Missing Sample',
      style: { transform: 'rotate(180deg)' },
    },
    {
      src: a11y ? fixed_waiting_accessible : fixed_waiting,
      label: 'Believed Fixed',
    },
    { src: a11y ? orange_triaged : red_triaged, label: 'Triaged Regression' },
    {
      src: a11y ? orange_3d_triaged : red_3d_triaged,
      label: 'Extreme Triaged Regression (>15%)',
    },
    { src: a11y ? orange : red, label: 'Significant Regression' },
    { src: a11y ? orange_3d : red_3d, label: 'Extreme Regression (>15%)' },
    { src: a11y ? fix_failed_accessible : fix_failed, label: 'Failed Fix' },
  ]

  return (
    <div
      style={{
        padding: '12px 16px',
        marginBottom: '12px',
        border: '1px solid #e0e0e0',
        borderRadius: '4px',
        textAlign: 'center',
      }}
    >
      <Typography
        variant="subtitle1"
        style={{ fontWeight: 'bold', marginBottom: '4px' }}
      >
        Status Key
      </Typography>
      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          gap: '12px 24px',
          padding: '4px 0',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        {items.map((item) => (
          <div
            key={item.label}
            style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
          >
            <img
              src={item.src}
              width="20px"
              height="20px"
              alt={item.label}
              style={item.style}
            />
            <Typography variant="body2">{item.label}</Typography>
          </div>
        ))}
      </div>
    </div>
  )
}

// The values of a column's key/value pairs (except status) are
// concatenated to form a column name
export function formColumnName(column) {
  let variants = column['variants']
  return Object.keys(variants)
    .map((key) => key + ':' + variants[key])
    .join(' ')
}

// Given the data pulled from the API server, calculate an array
// of columns using the first row.  Assumption: the number of columns
// is the same across all rows.
// A column looks like this and we concatenate all fields except status:
//   "columns": [
//       {
//         "network": "ovn",
//         "arch": "amd64",
//         "platform": "aws",
//         "status": 0
//       },
// Do our best to handle empty data or a "cancelled" condition.
export function getColumns(data) {
  if (!data || !data.rows || !data.rows[0] || !data.rows[0].component) {
    console.log(
      'data is one of: undefined, no rows, no rows[0], no row[0].component'
    )
    return ['No column']
  }
  if (data.rows[0].component == 'None' || !data.rows[0].columns) {
    return ['No data']
  }
  if (data.rows[0].component === 'Cancelled') {
    console.log('got cancelled')
    return ['Cancelled']
  }

  const firstColumn = data.rows[0].columns
  let columnNames = []
  firstColumn.forEach((column) => {
    const columnValues = formColumnName(column)
    columnNames.push(columnValues)
  })

  return columnNames
}

// The API likes RFC3339 times and the date pickers don't.  So we use this
// function to convert for when we call the API.
// 4 digits, followed by a -, followed by 2 digits, and so on all wrapped in
// a group so we can refer to them as $1 and $2 respectively.
// We add a 'T' in the middle and a 'Z' on the end.
export function makeRFC3339Time(aUrlStr) {
  // Translate all the %20 and %3a into spaces and colons so that the regex can work.
  const decodedStr = decodeURIComponent(aUrlStr)
  // URLSearchParams uses a + to separate date and time.
  const regex = /(\d{4}-\d{2}-\d{2})[\s+](\d{2}:\d{2}:\d{2})/g
  const replaceStr = '$1T$2Z'
  let retVal = decodedStr.replace(regex, replaceStr)

  // The api thinks that the null component is real and will filter accordingly
  // so omit it.
  retVal = retVal.replace(/&component=null/g, '')
  return retVal
}

// Return a formatted date given a long form date from the date picker.
// The given date can be either a long string (from the DatePicker),
// a number (epoch time from when we initialized the start times), or
// a Date object (when called from an event handler function).
//
// The format of the string can also vary, it could be an ISO8601 date from
// the API list views (2024-08-29T00:00:00Z), or the format we use for query params
// in component readiness (2024-09-05 23:59:59). It's very easy for these to parse into
// local time in the browser, then get truncated to the wrong date. To make things more fun
// unit tests will parse them as UTC. To work past this, the component readiness format gets
// an appended Z to force to UTC when parsing, keeping the function working consistently. (for now)
//
// TODO: ISO8601 *everywhere* might be good, I suspect we're bypassing some cache rounding the
// server tries to do.
export function formatLongDate(aLongDate, aDateFormat) {
  let dateObj
  const typeOfLongDate = typeof aLongDate
  if (typeOfLongDate == 'string' || typeOfLongDate == 'number') {
    if (typeOfLongDate == 'string' && !aLongDate.includes('Z')) {
      aLongDate += 'Z'
    }
    dateObj = new Date(aLongDate)
  } else if (typeOfLongDate == 'object') {
    dateObj = aLongDate
  } else {
    // This should never happen, but if it does, try to recover.
    console.log('Error: unknown date format: ', typeof aLongDate)
    dateObj = new Date(aLongDate)
  }
  let retVal = formatInTimeZone(dateObj, 'UTC', aDateFormat)
  return retVal
}

// These next set of variables are used for CompReadyMainInputs

// Take the values needed to make an api call and return a string that can be used to
// make that call.
// To be consistent, this value (filterVals) should not contain environment on any level
export function getUpdatedUrlParts(vars) {
  const valuesMap = {
    baseRelease: vars.baseRelease,
    baseStartTime: formatLongDate(vars.baseStartTime, dateFormat),
    baseEndTime: formatLongDate(vars.baseEndTime, dateEndFormat),
    sampleRelease: vars.sampleRelease,
    sampleStartTime: formatLongDate(vars.sampleStartTime, dateFormat),
    sampleEndTime: formatLongDate(vars.sampleEndTime, dateEndFormat),
    confidence: vars.confidence,
    pity: vars.pity,
    minFail: vars.minFail,
    passRateNewTests: vars.passRateNewTests,
    passRateAllTests: vars.passRateAllTests,
    ignoreDisruption: vars.ignoreDisruption,
    ignoreMissing: vars.ignoreMissing,
    flakeAsFailure: vars.flakeAsFailure,
    includeMultiReleaseAnalysis: vars.includeMultiReleaseAnalysis,
    //component: vars.component,
  }

  if (vars.samplePROrg && vars.samplePRRepo && vars.samplePRNumber) {
    valuesMap.samplePROrg = vars.samplePROrg
    valuesMap.samplePRRepo = vars.samplePRRepo
    valuesMap.samplePRNumber = vars.samplePRNumber
  }

  // TODO: inject the PR vars into query params

  function filterOutVariantCC(values) {
    return values.filter((value) => !vars.variantCrossCompare.includes(value))
  }
  const arraysMap = {
    columnGroupBy: filterOutVariantCC(vars.columnGroupByCheckedItems),
    dbGroupBy: filterOutVariantCC(vars.dbGroupByVariants),
  }

  const queryParams = new URLSearchParams()

  // Render the plain values first.
  Object.entries(valuesMap).forEach(([key, value]) => {
    queryParams.append(key, value)
  })

  // Render the array values.
  Object.entries(arraysMap).forEach(([key, value]) => {
    if (value && value.length) {
      queryParams.append(key, value.join(','))
    }
  })

  // Render selected variants
  convertVariantItemsToParam(vars.includeVariantsCheckedItems).forEach(
    (item) => {
      queryParams.append('includeVariant', item)
    }
  )
  Object.entries(vars.compareVariantsCheckedItems).forEach(
    ([group, variants]) => {
      // for UI purposes we may be holding compareVariants that aren't actually being compared, so they don't get wiped
      // out just by toggling the "Compare" button. But for the parameters we will filter these out.
      if (vars.variantCrossCompare.includes(group)) {
        variants.forEach((variant) => {
          queryParams.append('compareVariant', group + ':' + variant)
        })
      }
    }
  )
  vars.variantCrossCompare.forEach((item) => {
    queryParams.append('variantCrossCompare', item)
  })
  vars.samplePayloadTags.forEach((item) => {
    queryParams.append('samplePayloadTag', item)
  })
  vars.testCapabilities.forEach((item) => {
    queryParams.append('testCapabilities', item)
  })
  vars.testLifecycles.forEach((item) => {
    queryParams.append('testLifecycles', item)
  })

  // Stringify and put the begin param character.
  queryParams.sort() // ensure they always stay in sorted order to prevent url history changes

  // When using URLSearchParams to construct a query string, it follows the application/x-www-form-urlencoded format,
  // which uses + to represent space characters. The rest of Sippy uses the URI encoding tools in JS, which relies on
  // %20 for spaces. This makes URLs change, which creates additional history entries, and breaks the back button.
  const queryString = queryParams.toString().replace(/\+/g, '%20')
  return '?' + queryString
}

// sortQueryParams sorts a query parameters order so we don't screw up the history when they change
export function sortQueryParams(path) {
  // Split the path into base path and query string
  const [basePath, queryString] = path.split('?')

  if (!queryString) {
    return path
  }

  // Use URLSearchParams to parse and sort the query parameters
  const params = new URLSearchParams(queryString)
  const sortedParams = new URLSearchParams([...params.entries()].sort())

  // Re-assemble the path with sorted query parameters.
  // When using URLSearchParams to construct a query string, it follows the application/x-www-form-urlencoded format,
  // which uses + to represent space characters. The rest of Sippy uses the URI encoding tools in JS, which relies on
  // %20 for spaces. This makes URL's change, which creates additional history entries, and breaks the back button.
  return basePath + '?' + sortedParams.toString().replace(/\+/g, '%20')
}

// Single place to make titles so they look consistent as well as capture the
// key attributes you may want for debugging.
export function makePageTitle(title, ...args) {
  return (
    <Typography variant="h4" style={{ margin: 20, textAlign: 'center' }}>
      <div>{title}</div>
      {debugMode &&
        args.map((item, index) => (
          <div key={index}>
            <Typography variant="body2" component="div" key={index}>
              {item}
            </Typography>
          </div>
        ))}
      <hr />
    </Typography>
  )
}

// Given data and columnNames, calculate an array, parallel to columnNames,
// that has true or false depending on if that column is to be kept and displayed.
// The length and order of the returned array is identical to the columnNames array.
// The criteria for keeping a column is based on the redOnlyChecked checkbox.
// If redOnlyChecked is true, keep columns only if status <= -2
// If redOnlyChecked is false, keep all columns.
export function getKeeperColumns(data, columnNames, redOnlyChecked) {
  let keepColumnList = Array(columnNames.length).fill(
    redOnlyChecked ? false : true
  )

  if (!redOnlyChecked) {
    // All columns are kept and displayed.
    return keepColumnList
  }

  // Do a cross-sectional search across rows/componentsfor status <= -2
  data.rows.forEach((row) => {
    row.columns.forEach((column, index) => {
      // Only if status <= -2 do we keep/display this column.
      if (column.status <= -2) {
        keepColumnList[index] = true
      }
    })
  })
  return keepColumnList
}

export function validateData(data) {
  if (!data || !data.rows || !data.rows[0] || !data.rows[0].component) {
    console.log(
      'data is one of: undefined, no rows, no rows[0], no row[0].component'
    )
    return ['No data']
  }
  if (data.rows[0].component === 'None' || !data.rows[0].columns) {
    return ['No data']
  }
  if (data.rows[0].component === 'Cancelled') {
    console.log('got cancelled')
    return ['Cancelled']
  }

  return ['']
}

// mergeRegressionData takes the data from CR api and organizes the data
// for regressedTests (untriaged), combined list of untriaged and triaged tests
// all used in the RegressedTestsModal dialog
// if there is a triage entry with a matching regression_id, it computes the proper status for the triaged icon, and removes the corresponding explanations
export function mergeRegressionData(data, triageEntries) {
  let ret = validateData(data)
  if (ret[0] !== '') {
    // Return empty arrays when data is invalid to maintain expected structure
    return {
      untriagedRegressedTests: [],
      allRegressions: [],
      unresolvedRegressedTests: [],
    }
  }

  // the set of regressionIds from the triageEntries will be used to determine if a test that has other not been triaged should be marked as such
  const regressionIds = new Set()
  triageEntries.forEach((tr) => {
    tr.regressions.forEach((regression) => {
      regressionIds.add(regression.id)
    })
  })

  let untriagedRegressedTests = []
  let allRegressions = []
  let unresolvedRegressedTests = []

  data.rows.forEach((row) => {
    row.columns.forEach((column) => {
      const regressed = column.regressed_tests
      if (column.regressed_tests && regressed.length > 0) {
        regressed.forEach((r) => {
          if (!regressionIds.has(r.regression?.id)) {
            untriagedRegressedTests.push(r)
          }
        })
        allRegressions = allRegressions.concat(regressed)
      }
    })
  })

  allRegressions.forEach((r) => {
    // Anything below "hopefully fixed" belongs in unresolvedRegressedTests
    if (r.status <= -200) {
      unresolvedRegressedTests.push(r)
    }
  })

  untriagedRegressedTests.sort((a, b) => {
    return (
      a.component.toLowerCase() < b.component.toLowerCase() ||
      a.capability.toLowerCase() < b.capability.toLowerCase()
    )
  })
  untriagedRegressedTests = untriagedRegressedTests.map((item, index) => ({
    ...item,
    id: index,
  }))

  allRegressions.sort((a, b) => {
    return (
      a.component.toLowerCase() < b.component.toLowerCase() ||
      a.capability.toLowerCase() < b.capability.toLowerCase()
    )
  })
  allRegressions = allRegressions.map((item, index) => ({ ...item, id: index }))

  return {
    untriagedRegressedTests: untriagedRegressedTests,
    allRegressions: allRegressions,
    unresolvedRegressedTests: unresolvedRegressedTests,
  }
}

export const Search = styled('div')(({ theme }) => ({
  position: 'relative',
  borderRadius: theme.shape.borderRadius,
  backgroundColor: alpha(theme.palette.common.white, 0.15),
  '&:hover': {
    backgroundColor: alpha(theme.palette.common.white, 0.25),
  },
  marginRight: theme.spacing(2),
  marginLeft: 0,
  width: '100%',
  [theme.breakpoints.up('sm')]: {
    marginLeft: theme.spacing(0),
    width: 'auto',
  },
}))

export const SearchIconWrapper = styled('div')(({ theme }) => ({
  padding: theme.spacing(0, 2),
  height: '100%',
  position: 'absolute',
  pointerEvents: 'none',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
}))

export const StyledInputBase = styled(InputBase)(({ theme }) => ({
  color: 'inherit',
  '& .MuiInputBase-input': {
    padding: theme.spacing(1, 1, 1, 0),
    // vertical padding + font size from searchIcon
    paddingLeft: `calc(1em + ${theme.spacing(4)})`,
    transition: theme.transitions.create('width'),
    width: '100%',
    [theme.breakpoints.up('md')]: {
      width: '20ch',
    },
  },
}))

/** some functions for managing the variant parameters **/
export const convertParamToVariantItems = (variantItemsParam) => {
  let groupedVariants = {}
  variantItemsParam.forEach((variant) => {
    // each variant here should look like e.g. "Platform:aws"
    let kv = variant.split(':')
    if (kv.length === 2) {
      if (kv[0] in groupedVariants) {
        groupedVariants[kv[0]].push(kv[1])
      } else {
        groupedVariants[kv[0]] = [kv[1]]
      }
    }
  })
  return groupedVariants
  // which now looks like {Platform: ["aws", "azure"], ...}
}
export const convertVariantItemsToParam = (groupedVariants) => {
  let param = []
  Object.keys(groupedVariants).forEach((group) => {
    groupedVariants[group].forEach((variant) => {
      param.push(group + ':' + variant)
    })
  })
  return param
}

// Convert API URL to a relative UI URL by stripping any scheme+host and
// swapping the /api/ prefix for /sippy-ng/.  Producing a relative path
// avoids the bug where the backend embeds localhost (or another internal
// host) as the origin in HATEOAS links.
export function convertApiUrlToUiUrl(apiUrl) {
  const apiIndex = apiUrl.indexOf('/api/')
  if (apiIndex === -1) {
    return apiUrl
  }
  const pathAndQuery = apiUrl.substring(apiIndex)
  if (pathAndQuery.startsWith('/api/component_readiness/')) {
    return pathAndQuery.replace(
      '/api/component_readiness/',
      '/sippy-ng/component_readiness/'
    )
  }
  return pathAndQuery.replace('/api/', '/sippy-ng/')
}

// Extracts the test_details link from HATEOAS links. Prefers the plain
// "test_details" key (used by regressed tests in component reports), then
// tries "test_details:<viewName>" if a viewName is given, then falls back
// to the first "test_details:*" composite key (used by regression objects).
export function getTestDetailsLink(links, viewName) {
  if (!links) return null
  if (links['test_details']) return links['test_details']
  if (viewName) {
    return links[`test_details:${viewName}`] || null
  }
  const key = Object.keys(links).find((k) => k.startsWith('test_details:'))
  return key ? links[key] : null
}

export function generateTestDetailsReportLink(test, viewName) {
  const testDetailsUrl = getTestDetailsLink(test.links, viewName)
  if (testDetailsUrl) {
    return convertApiUrlToUiUrl(testDetailsUrl)
  }
  return null
}

const SYMPTOM_COLORS = [
  '#1976d2',
  '#d32f2f',
  '#388e3c',
  '#f57c00',
  '#7b1fa2',
  '#0097a7',
  '#c2185b',
  '#455a64',
  '#5d4037',
  '#303f9f',
]

export function symptomColor(symptomId) {
  let hash = 0
  for (let i = 0; i < symptomId.length; i++) {
    hash = (hash * 31 + symptomId.charCodeAt(i)) | 0
  }
  return SYMPTOM_COLORS[Math.abs(hash) % SYMPTOM_COLORS.length]
}

// Helper function to check if triage has any regressions with status -1000 (failed fix).
// allRegressedTests is a map of view name to array of regressed tests.
export function hasFailedFixRegression(triage, allRegressedTests) {
  const tests = allRegressedTests ? Object.values(allRegressedTests).flat() : []
  if (!tests.length || !triage.regressions) {
    return false
  }

  // Get regression IDs from this triage
  const triageRegressionIds = triage.regressions.map((r) => r.id)

  // Filter tests to find those matching this triage's regressions
  const relevantRegressedTests = tests.filter(
    (rt) => rt?.regression?.id && triageRegressionIds.includes(rt.regression.id)
  )

  // Check if any have status -1000
  return relevantRegressedTests.some((rt) => rt.status === -1000)
}
