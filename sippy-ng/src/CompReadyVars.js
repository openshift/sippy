import { debugMode } from './component_readiness/CompReadyUtils'
import { getAPIUrl } from './component_readiness/CompReadyUtils'
import { safeEncodeURIComponent } from './helpers'
import CompReadyProgress from './component_readiness/CompReadyProgress'
import PropTypes from 'prop-types'
import React, { createContext, useEffect, useState } from 'react'
export const CompReadyVarsContext = createContext()

export const CompReadyVarsProvider = ({ children }) => {
  const [excludeNetworksList, setExcludeNetworksList] = useState([])
  const [excludeCloudsList, setExcludeCloudsList] = useState([])
  const [excludeArchesList, setExcludeArchesList] = useState([])
  const [excludeUpgradesList, setExcludeUpgradesList] = useState([])
  const [excludeVariantsList, setExcludeVariantsList] = useState([])
  const [isLoaded, setIsLoaded] = useState(false)

  useEffect(() => {
    console.log('loading vars')
    fetch(getAPIUrl() + '/variants')
      .then((response) => response.json())
      .then((data) => {
        if (data.platform.length < 1) {
          console.log('---- data.platform has a length less than 1')
        }
        if (data.arch.length < 1) {
          console.log('  ---- data.arch has a length less than 1')
        }
        if (data.network.length < 1) {
          console.log('    ---- data.network has a length less than 1')
        }
        if (data.upgrade.length < 1) {
          console.log('      ---- data.upgrade has a length less than 1')
        }
        if (data.variant.length < 1) {
          console.log(' .      ---- data.variant has a length less than 1')
        }

        console.log('(((( got variants ))))')
        setExcludeCloudsList(data.platform)
        setExcludeArchesList(data.arch)
        setExcludeNetworksList(data.network)
        setExcludeUpgradesList(data.upgrade)
        setExcludeVariantsList(data.variant)
        setIsLoaded(true)
        console.log('done loading vars')
      })
      .catch((error) =>
        console.error('Error loading variables via sippy api', error)
      )
  }, [])

  // Take a string that is an "environment" (environment is a list of strings that describe
  // items in one or more of the lists above) and split it up so that it can be used in
  // an api call.  We keep this concept of "environment" because it's used for column labels.
  const expandEnvironment = (environmentStr) => {
    if (debugMode) {
      if (
        excludeNetworksList.length < 1 &&
        excludeCloudsList.length < 1 &&
        excludeArchesList.length < 1 &&
        excludeUpgradesList.length < 1 &&
        excludeVariantsList.length < 1
      ) {
        console.log('*** All are less than 1')
      } else {
        if (excludeNetworksList.length < 1) {
          console.log('excludeNetworksList has a length less than 1')
        }
        if (excludeCloudsList.length < 1) {
          console.log('  excludeCloudsList has a length less than 1')
        }
        if (excludeArchesList.length < 1) {
          console.log('    excludeArchesList has a length less than 1')
        }
        if (excludeUpgradesList.length < 1) {
          console.log('      excludeUpgradesList has a length less than 1')
        }
        if (excludeVariantsList.length < 1) {
          console.log('        excludeVariantsList has a length less than 1')
        }
      }
    }

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
    // TODO: this button will do nothing for now
    console.log('Aborting ...')
  }

  // We do this just like any other fetch to ensure the variables get loaded from
  // the sippy API before any consumers use those variables or the expandEnvironment
  // function that depends on them.I think
  if (!isLoaded) {
    return <CompReadyProgress apiLink={'none'} cancelFunc={cancelFetch} />
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
      }}
    >
      {children}
    </CompReadyVarsContext.Provider>
  )
}

CompReadyVarsProvider.propTypes = {
  children: PropTypes.node.isRequired,
}
