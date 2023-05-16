import { Link } from 'react-router-dom'
import PropTypes from 'prop-types'
import React from 'react'

// Use this to print the title of the pages and the accompanying api call string
// for debugging.  Later, we can remove the part that prints the curl.
export default function CompReadyPageTitle(props) {
  const { pageTitle, apiCallStr } = props
  const callStr = `${apiCallStr}`

  return (
    <div>
      {pageTitle}
      <p>curl -sk &apos;{callStr}&apos;</p>
    </div>
  )
}

CompReadyPageTitle.propTypes = {
  pageTitle: PropTypes.string.isRequired,
  apiCallStr: PropTypes.string.isRequired,
}
