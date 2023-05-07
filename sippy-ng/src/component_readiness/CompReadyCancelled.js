import { Link } from 'react-router-dom'
import React from 'react'

export default function CompReadyCancelled() {
  return (
    <div>
      <p>Operation cancelled or no data</p>
      <button>
        <Link to="/component_readiness">Start Over</Link>
      </button>
    </div>
  )
}
