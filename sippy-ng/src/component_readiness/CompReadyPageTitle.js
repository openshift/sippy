import { debugMode } from './CompReadyUtils'
import PropTypes from 'prop-types'
import React from 'react'

// CompReadyPageTitle is used to print the title of the pages and the accompanying
// api call string for debugging.  This allows us to have consistent page titles
// and on page1, we can see the api call string change dynamically.
// The call string is also embedded, but not displayed, so that it can still be
// retrieved without a modified version of sippy (where debugMode is set to true).
// The "callStr" id is there so that you can run:
//   const x = document.getElementById('callStr'); x.innerText
// in the javascript console to see the value.
// We can remove the part that prints the curl once debugging is no longer needed.
export default function CompReadyPageTitle(props) {
  const { pageTitle, apiCallStr } = props
  const callStr = `${apiCallStr}`

  return (
    <div>
      {pageTitle}
      {debugMode ? <p>curl -sk &apos;{callStr}&apos;</p> : null}
      <div id="callStr" style={{ display: 'none' }}>
        <p>{callStr}</p>
      </div>
    </div>
  )
}

CompReadyPageTitle.propTypes = {
  pageTitle: PropTypes.object.isRequired,
  apiCallStr: PropTypes.string.isRequired,
}
