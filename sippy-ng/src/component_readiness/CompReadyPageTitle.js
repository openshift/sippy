import PropTypes from 'prop-types'
import React from 'react'

// CompReadyPageTitle is used to print the title of the pages and the accompanying
// api call string for debugging.  This allows us to have consistent page titles
// and on page1, we can see the api call string change dynamically.
// We can remove the part that prints the curl once debugging is no longer needed.
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
  pageTitle: PropTypes.object.isRequired,
  apiCallStr: PropTypes.string.isRequired,
}
