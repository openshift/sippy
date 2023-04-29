import { Link } from 'react-router-dom'
import { makeRFC3339Time } from '../helpers'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

// Take an url for the browser and transform it into another url suitable for
// calling the api
function urlToApiCall(aUrlStr) {
  console.log('urlToApi in: ', aUrlStr)
  const retVal = aUrlStr.replace(
    '/componentreadiness/tests/',
    '/component_readiness/'
  )
  console.log('retVal: ', retVal)
  return retVal
}

export default function CompReadyTest(props) {
  const { filterVals } = props

  const uiUrl = 'localhost:3000'
  const apiUrl = 'localhost:8080/api/component_readiness'
  document.title = `CompRead Test`
  const urlParams = new URLSearchParams(decodeURIComponent(location.search))
  const comp = urlParams.get('component')
  const env = urlParams.get('environment')
  console.log('filterVals T: ', filterVals)
  const t = makeRFC3339Time(filterVals)
  console.log('filterVals T decoded: ', t)
  return (
    <Fragment>
      <h1>
        <Link to="/componentreadiness">/</Link> {env} &gt; {comp}
      </h1>
      <br></br>
      <h2>
        <a
          href={
            'http://localhost:8080/api/component_readiness' +
            makeRFC3339Time(filterVals)
          }
        >
          apiCall
        </a>
      </h2>
    </Fragment>
  )
}

CompReadyTest.propTypes = {
  filterVals: PropTypes.string.isRequired,
}
