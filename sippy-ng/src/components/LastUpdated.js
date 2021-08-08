import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function LastUpdated (props) {
  const minute = 1000 * 60 // Milliseconds in a minute
  const hour = 60 * minute // Milliseconds in an hour
  const millisAgo = props.lastUpdated ? (props.lastUpdated.getTime() - Date.now()) : 0

  try {
    if (millisAgo === 0) {
      return <Fragment>Last update unknown</Fragment>
    } else if (Math.abs(millisAgo) < hour) {
      return (
                <Fragment>
                    Last updated {Math.round(Math.abs(millisAgo) / minute) + ' minutes ago'}
                </Fragment>
      )
    } else {
      return (
                <Fragment>
                    Last updated {Math.round(Math.abs(millisAgo) / hour) + ' hours ago'}
                </Fragment>
      )
    }
  } catch (e) {
    return <></>
  }
}

LastUpdated.propTypes = {
  lastUpdated: PropTypes.instanceOf(Date).isRequired
}
