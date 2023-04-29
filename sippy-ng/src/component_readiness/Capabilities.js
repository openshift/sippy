import PropTypes from 'prop-types'
import React from 'react'

export default function Capabilities(props) {
  document.title = `Capabilities`
  return <h1>Capabilities page for: {props.component}</h1>
}

Capabilities.propTypes = {
  component: PropTypes.string,
}
