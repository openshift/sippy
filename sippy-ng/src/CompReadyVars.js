import { getAPIUrl } from './component_readiness/CompReadyUtils'
import { gotFetchError } from './component_readiness/CompReadyUtils'
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
  const [fetchError, setFetchError] = useState('')

  useEffect(() => {
    fetch(getAPIUrl() + '/variants')
      .then((response) => response.json())
      .then((data) => {
        setExcludeCloudsList(data.platform)
        setExcludeArchesList(data.arch)
        setExcludeNetworksList(data.network)
        setExcludeUpgradesList(data.upgrade)
        setExcludeVariantsList(data.variant)
        setIsLoaded(true)
      })
      .catch((error) => {
        setFetchError('Error loading /variant variables via sippy API', error)
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
      }}
    >
      {children}
    </CompReadyVarsContext.Provider>
  )
}

CompReadyVarsProvider.propTypes = {
  children: PropTypes.node.isRequired,
}
