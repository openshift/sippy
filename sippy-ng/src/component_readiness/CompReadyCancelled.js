import { gotoCompReadyMain } from './CompReadyUtils'
import PropTypes from 'prop-types'
import React from 'react'

export default function CompReadyCancelled(props) {
  const { message, apiCallStr } = props
  const outStr = apiCallStr.split('&')
  return (
    <div>
      <p>
        {message === 'None'
          ? `This yields no data: curl -sk '${apiCallStr}'`
          : `Operation Cancelled: ${message}`}
      </p>
      <ul>
        {outStr.map((item, index) => {
          if (!item.endsWith('=')) {
            return <li key={index}>{item}</li>
          } else {
            return null
          }
        })}
      </ul>
      <button onClick={gotoCompReadyMain}>Start Over</button>
    </div>
  )
}

CompReadyCancelled.propTypes = {
  message: PropTypes.string.isRequired,
  apiCallStr: PropTypes.string.isRequired,
}
