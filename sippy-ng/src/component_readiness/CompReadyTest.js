import { Link } from 'react-router-dom'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function CompReadyTest(props) {
  const { filterVals } = props

  document.title = `CompRead Test`
  const urlParams = new URLSearchParams(decodeURIComponent(location.search))
  const comp = urlParams.get('component')
  const env = urlParams.get('environment')
  return (
    <Fragment>
      <h1>
        Test page for {comp}, {env}
      </h1>
      filterVals={filterVals}
      <br></br>
      <Link to="">apiCall</Link>
    </Fragment>
  )
}

CompReadyTest.propTypes = {
  filterVals: PropTypes.string.isRequired,
}
