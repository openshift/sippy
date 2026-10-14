import { convertApiUrlToUiUrl } from './CompReadyUtils'
import { Link } from 'react-router'
import PropTypes from 'prop-types'
import React from 'react'

export default function LatestReportLink({ apiUrl }) {
  const latestReportUrl = convertApiUrlToUiUrl(apiUrl)
  const routerUrl = latestReportUrl.replace(/^\/sippy-ng(?=\/|$)/, '')

  return (
    <Link to={routerUrl} style={{ textDecoration: 'underline' }}>
      View the latest report with data from the last 7 days
    </Link>
  )
}

LatestReportLink.propTypes = {
  apiUrl: PropTypes.string.isRequired,
}
