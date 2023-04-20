import PropTypes from 'prop-types'
import React from 'react'

export default function CompReadyTest(props) {
  return <h1>Test page for: {props.columnName}</h1>
}

CompReadyTest.propTypes = {
  columnName: PropTypes.string,
}
