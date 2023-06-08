import { Alert } from '@material-ui/lab'
import { CompReadyVarsContext } from '../CompReadyVars'
import { format } from 'date-fns'
import { safeEncodeURIComponent } from '../helpers'
import { Typography } from '@material-ui/core'
import green from './green-3.png'
import green_half_data from './green-half-data.png'
import green_missing_data from './green_no_data.png'
import heart from './green-heart.png'
import React, { useContext } from 'react'
import red from './red-3.png'
import red_3d from './red-3d.png'

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
        },
      ],
    },
  ],
}
// Make one place to create the Component Readiness api call
export function getAPIUrl() {
  return process.env.REACT_APP_API_URL + '/api/component_readiness'
}

// Make one place to create the Component Readiness test_details api call
export function getTestDetailsAPIUrl() {
  return process.env.REACT_APP_API_URL + '/api/component_readiness/test_details'
}

export const gotoCompReadyMain = () => {
  window.location.href = '/sippy-ng/component_readiness/main'
  //window.history.back()
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
export function getStatusAndIcon(status, grayFactor = 0) {
  let icon = ''

  let statusStr = status + ': '

  if (status >= 3) {
    statusStr =
      statusStr + 'SignificantImprovement detected (improved sample rate)'
    icon = (
      <img
        src={heart}
        width="20px"
        height="20px"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status == 2) {
    statusStr =
      statusStr + 'Missing Basis And Sample (basis and sample data missing)'
    icon = (
      <img
        src={green_missing_data}
        alt="MissingBasisAndSample"
        width="15px"
        height="15px"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status == 1) {
    statusStr = statusStr + 'Missing Basis (basis data missing)'
    icon = (
      <img
        src={green_half_data}
        alt="MissingBasis"
        width="15px"
        height="15px"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status == 0) {
    statusStr = statusStr + 'NoSignificantDifference detected'
    icon = (
      <img
        src={green}
        alt="NotSignificant"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status == -1) {
    statusStr = statusStr + 'Missing Sample (sample data missing)'
    icon = (
      <img
        src={green_missing_data}
        alt="MissingBasisAndSample"
        width="15px"
        height="15px"
        style={{ filter: `grayscale(${grayFactor}%)` }}
      />
    )
  } else if (status == -2) {
    statusStr = statusStr + 'SignificantRegression detected'
    icon = <img src={red} alt="SignificantRegression" />
  } else if (status <= -3) {
    statusStr =
      statusStr + 'ExtremeRegression detected ( >15% pass rate change)'
    icon = <img src={red_3d} alt="ExtremRegressio >15%n" />
  }

  return [statusStr, icon]
}

// The values of a column's key/value pairs (except status) are
// concatenated to form a column name
export function formColumnName(column) {
  return Object.keys(column)
    .filter((key) => key != 'status')
    .map((key) => column[key])
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
//         "platform": "alibaba",
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
export function formatLongDate(aLongDateStr) {
  const dateObj = new Date(aLongDateStr)
  const ret = format(dateObj, dateFormat)
  return ret
}

export function formatLongEndDate(aLongDateStr) {
  const dateObj = new Date(aLongDateStr)
  const ret = format(dateObj, dateEndFormat)
  return ret
}

// These next set of variables are used for CompReadyMainInputs

export const groupByList = ['cloud', 'arch', 'network', 'upgrade', 'variants']




// Take the values needed to make an api call and return a string that can be used to
// make that call.
export function getUpdatedUrlParts(
  baseRelease,
  baseStartTime,
  baseEndTime,
  sampleRelease,
  sampleStartTime,
  sampleEndTime,
  groupByCheckedItems,
  excludeCloudsCheckedItems,
  excludeArchesCheckedItems,
  excludeNetworksCheckedItems,
  excludeUpgradesCheckedItems,
  excludeVariantsCheckedItems,
  confidence,
  pity,
  minFail,
  ignoreDisruption,
  ignoreMissing
) {
  const valuesMap = {
    baseRelease: baseRelease,
    baseStartTime: formatLongDate(baseStartTime),
    baseEndTime: formatLongEndDate(baseEndTime),
    sampleRelease: sampleRelease,
    sampleStartTime: formatLongDate(sampleStartTime),
    sampleEndTime: formatLongEndDate(sampleEndTime),
    confidence: confidence,
    pity: pity,
    minFail: minFail,
    ignoreDisruption: ignoreDisruption,
    ignoreMissing: ignoreMissing,
    //component: component,
  }

  const arraysMap = {
    excludeClouds: excludeCloudsCheckedItems,
    excludeArches: excludeArchesCheckedItems,
    excludeNetworks: excludeNetworksCheckedItems,
    excludeUpgrades: excludeUpgradesCheckedItems,
    excludeVariants: excludeVariantsCheckedItems,
    groupBy: groupByCheckedItems,
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

  // Stringify and put the begin param character.
  const queryString = queryParams.toString()
  const retVal = `?${queryString}`
  return retVal
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
