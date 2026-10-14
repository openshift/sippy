import { convertApiUrlToUiUrl } from './CompReadyUtils'
import { Link } from 'react-router'
import PropTypes from 'prop-types'
import React from 'react'

export default function LatestReportLink({ apiUrl }) {
  const routerUrl = convertApiUrlToUiUrl(apiUrl, '')

  return (
    <Link to={routerUrl} style={{ textDecoration: 'underline' }}>
      View the latest report with data from the last 7 days
    </Link>
  )
}

LatestReportLink.propTypes = {
  apiUrl: PropTypes.string.isRequired,
}
