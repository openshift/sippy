import { Link } from 'react-router-dom'
import PropTypes from 'prop-types'
import React from 'react'

export default function CompReadyCancelled(props) {
  const { message, apiCallStr } = props
  return (
    <div>
      <p>
        {message == 'None'
          ? `This yields no data: curl -sk '${apiCallStr}'`
          : `Operation Cancelled: ${message}`}
      </p>
      <button>
        <Link to="/component_readiness">Start Over</Link>
      </button>
    </div>
  )
}

CompReadyCancelled.propTypes = {
  message: PropTypes.string.isRequired,
  apiCallStr: PropTypes.string.isRequired,
}
