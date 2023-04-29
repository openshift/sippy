import { format } from 'date-fns'
import { safeEncodeURIComponent } from '../helpers'

export const dateFormat = 'yyyy-MM-dd HH:mm:ss'

// Make one place to create the Component Readiness api call
export function getAPIUrl() {
  const mainUrl = window.location.host.split(':')[0]

  console.log('mainUrl: ', mainUrl)
  return mainUrl + ':8080/api/component_readiness'
}

// The API likes RFC3339 times and the date pickers don't.  So we use this
// function to convert for when we call the API.
// 4 digits, followed by a -, followed by 2 digits, and so on all wrapped in
// a group so we can refer to them as $1 and $2 respectively.
// We add a 'T' in the middle and a 'Z' on the end.
export function makeRFC3339Time(aUrlStr) {
  // Translate all the %20 and %3a into spaces and colons so that the regex can work.
  console.log('rfc anUrlStr: ', aUrlStr)
  const decodedStr = decodeURIComponent(aUrlStr)
  console.log('decodedStr:', decodedStr)
  const regex = /(\d{4}-\d{2}-\d{2})\s(\d{2}:\d{2}:\d{2})/g
  const replaceStr = '$1T$2Z'
  let retVal = decodedStr.replace(regex, replaceStr)
  retVal = retVal.replace(/component=\[(.*?)\]/g, 'component=$1')
  console.log('rfc retVal: ', retVal)
  return retVal
}

// Return a formatted date given a long form date from the date picker.
function formatLongDate(aLongDateStr) {
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
  console.log('getUpdatedUrlParts()')
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
