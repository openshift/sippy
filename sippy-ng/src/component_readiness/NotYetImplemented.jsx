import { useNavigate } from 'react-router-dom'
import PropTypes from 'prop-types'
import React from 'react'

export default function NotYetImplemented(props) {
  const { path } = props
  const navigate = useNavigate()

  const goBack = () => {
    history.goBack()
  }

  return (
    <div>
      <h2>({path}): Not Yet Implemented</h2>
      <button onClick={goBack}>Go Back</button>
    </div>
  )
}

NotYetImplemented.propTypes = {
  path: PropTypes.string.isRequired,
}
