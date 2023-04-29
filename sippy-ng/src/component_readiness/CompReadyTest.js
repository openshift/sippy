import { getAPIUrl, makeRFC3339Time } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

// Take an url for the browser and transform it into another url suitable for
// calling the api
function urlToApiCall(aUrlStr) {
  console.log('urlToApi in: ', aUrlStr)

  // Do some transformations here.
  const retVal = aUrlStr
  console.log('retVal: ', retVal)
  return retVal
}

export default function CompReadyTest(props) {
  const { filterVals } = props

  document.title = `CompRead Test`
  const urlParams = new URLSearchParams(decodeURIComponent(location.search))
  const comp = urlParams.get('component')
  const env = urlParams.get('environment')
  console.log('filterVals T: ', filterVals)
  const t = makeRFC3339Time(filterVals)

  let envStr = '&environment=' + env
  if (filterVals.includes('environment')) {
    let envStr = ''
  }
  const apiCallStr = getAPIUrl() + makeRFC3339Time(filterVals + envStr)
  console.log('filterVals T decoded: ', t)
  return (
    <Fragment>
      <h1>
        <Link to="/component_readiness">/</Link> {env} &gt; {comp}
      </h1>
      <br></br>
      <h2>
        <a href={apiCallStr}>{apiCallStr}</a>
      </h2>
    </Fragment>
  )
}

CompReadyTest.propTypes = {
  filterVals: PropTypes.string.isRequired,
}
