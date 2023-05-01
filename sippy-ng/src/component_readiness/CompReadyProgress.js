import { CircularProgress } from '@material-ui/core'
import { Fragment } from 'react'
import Button from '@material-ui/core/Button'
import PropTypes from 'prop-types'
import React from 'react'

export default function CompReadyProgress(props) {
  const { apiLink, cancelFunc } = props
  return (
    <Fragment>
      <p>
        Loading component readiness data ... If you asked for a huge dataset, it
        may take minutes.
      </p>
      <br />
      Here is the API call in case you are interested:
      <br />
      <h3>
        <a href={apiLink}>{apiLink}</a>
      </h3>
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
