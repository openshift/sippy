import { getAPIUrl } from './component_readiness/CompReadyUtils'
import PropTypes from 'prop-types'
import React, { createContext, useEffect, useState } from 'react'
export const CompReadyVarsContext = createContext()

export const CompReadyVarsProvider = ({ children }) => {
  const [excludeNetworksList, setExcludeNetworksList] = useState([])
  const [excludeCloudsList, setExcludeCloudsList] = useState([])
  const [excludeArchesList, setExcludeArchesList] = useState([])
  const [excludeUpgradesList, setExcludeUpgradesList] = useState([])
  const [excludeVariantsList, setExcludeVariantsList] = useState([])

  useEffect(() => {
    fetch(getAPIUrl() + '/variants')
      .then((response) => response.json())
      .then((data) => {
        setExcludeCloudsList(data.platform)
        setExcludeArchesList(data.arch)
        setExcludeNetworksList(data.network)
        setExcludeUpgradesList(data.upgrade)
        setExcludeVariantsList(data.variant)
      })
      .catch((error) =>
        console.error('Error loading variables via sippy api', error)
      )
  }, [])

  return (
    <CompReadyVarsContext.Provider
      value={{
        excludeNetworksList,
        excludeCloudsList,
        excludeArchesList,
        excludeUpgradesList,
        excludeVariantsList,
      }}
    >
      {children}
    </CompReadyVarsContext.Provider>
  )
}

CompReadyVarsProvider.propTypes = {
  children: PropTypes.node.isRequired,
}
