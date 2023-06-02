import { CircularProgress } from '@material-ui/core'
import { Fragment } from 'react'
import Button from '@material-ui/core/Button'
import PropTypes from 'prop-types'
import React from 'react'

// Dump out the api call in a nice format to help debug and give
// the user what we're asking for in case they can spot something
// wrong.
export default function CompReadyProgress(props) {
  const { apiLink, cancelFunc } = props
  const currentTitle = document.title

  // Make the title different so you can tell it's loading
  document.title = 'Loading ...'

  return (
    <Fragment>
      <p>
        Loading component readiness data ... If you asked for a huge dataset, it
        may take minutes.
      </p>
      <br />
      Here is the clickable API call in case you are interested:
      <br />
      <a href={apiLink}>
        {
          <ul>
            {apiLink.split('&').map((item, index) => {
              if (!item.endsWith('=')) {
                return <li key={index}>{item}</li>
              } else {
                return null
              }
            })}
          </ul>
        }
      </a>
      <CircularProgress />
      <div>
        <Button
          size="medium"
          variant="contained"
          color="secondary"
          onClick={cancelFunc}
        >
          Cancel
        </Button>
      </div>
    </Fragment>
  )
}

CompReadyProgress.propTypes = {
  apiLink: PropTypes.string.isRequired,
  cancelFunc: PropTypes.func.isRequired,
}
