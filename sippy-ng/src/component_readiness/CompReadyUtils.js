import { format } from 'date-fns'
import { safeEncodeURIComponent } from '../helpers'

// Make the HH:mm:ss as zeros to be more conducive to caching query caching.
export const dateFormat = 'yyyy-MM-dd 00:00:00'

// Make one place to create the Component Readiness api call
export function getAPIUrl() {
  const mainUrl = window.location.host.split(':')[0]

  //console.log('mainUrl: ', mainUrl)
  return 'http://' + mainUrl + ':8080/api/component_readiness'
}

// This is used when the user clicks on one of the columns at the top of the table
// Not quite used just yet.
export function singleRowReport(columnName) {
  return '/component_readiness/' + safeEncodeURIComponent(columnName) + '/tests'
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
export function getColumns(data) {
  const row0Columns = data.rows[0].columns

  let retVal = []
  row0Columns.forEach((column) => {
    let columnName = ''
    for (const key in column) {
      if (key !== 'status') {
        columnName = columnName + ' ' + column[key]
      }
    }
    retVal.push(columnName.trimStart())
  })
  return retVal
}

// The API likes RFC3339 times and the date pickers don't.  So we use this
// function to convert for when we call the API.
// 4 digits, followed by a -, followed by 2 digits, and so on all wrapped in
// a group so we can refer to them as $1 and $2 respectively.
// We add a 'T' in the middle and a 'Z' on the end.
export function makeRFC3339Time(aUrlStr) {
  // Translate all the %20 and %3a into spaces and colons so that the regex can work.
  //console.log('rfc anUrlStr: ', aUrlStr)
  const decodedStr = decodeURIComponent(aUrlStr)
  const regex = /(\d{4}-\d{2}-\d{2})\s(\d{2}:\d{2}:\d{2})/g
  const replaceStr = '$1T$2Z'
  let retVal = decodedStr.replace(regex, replaceStr)

  // curl doesn't like the brackets (you need to escape them).
  // TODO see if the api is ok with them.
  //retVal = retVal.replace(/component=\[(.*?)\]/g, 'component=\\[$1\\]')

  // The api thinks that the null component is real and will filter accordingly
  // so omit it.
  retVal = retVal.replace(/&component=null/g, '')
  //console.log('rfc retVal: ', retVal)
  return retVal
}

// Return a formatted date given a long form date from the date picker.
export function formatLongDate(aLongDateStr) {
  const dateObj = new Date(aLongDateStr)
  const ret = format(dateObj, dateFormat)
  //console.log('formatLongDate: ', ret)
  return ret
}

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
  component,
  environment
) {
  //console.log('getUpdatedUrlParts()')
  const valuesMap = {
    baseRelease: baseRelease,
    baseStartTime: formatLongDate(baseStartTime),
    baseEndTime: formatLongDate(baseEndTime),
    sampleRelease: sampleRelease,
    sampleStartTime: formatLongDate(sampleStartTime),
    sampleEndTime: formatLongDate(sampleEndTime),
    component: component,
  }

  const arraysMap = {
    exclude_clouds: excludeCloudsCheckedItems,
    exclude_arches: excludeArchesCheckedItems,
    exclude_networks: excludeNetworksCheckedItems,
    exclude_upgrades: excludeUpgradesCheckedItems,
    exclude_variants: excludeVariantsCheckedItems,
    group_by: groupByCheckedItems,
  }

  // Render the plain values first.
  let retVal = '?'
  let fieldList1 = Object.entries(valuesMap)
  fieldList1.map(([key, value]) => {
    let amper = '&'
    if (key === 'baseRelease') {
      amper = ''
    }
    retVal = retVal + amper + key + '=' + safeEncodeURIComponent(value)
  })

  const fieldList = Object.entries(arraysMap)
  fieldList.map(([key, value]) => {
    retVal = retVal + '&' + key + '='
    let first = true

    // Account for the case where value is undefined
    // because the url said something like exclude_clouds=, ...
    if (value) {
      value.map((item) => {
        let comma = ','
        if (first) {
          comma = ''
          first = false
        }
        retVal = retVal + comma + item
      })
    }
  })
  return retVal
}
