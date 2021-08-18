import { Button, Grid, TextField } from '@material-ui/core'
import React, { Fragment } from 'react'
import PropTypes from 'prop-types'

/**
 * FilterBox creates a TextField and submit link for filtering
 * a set of results.
 */
export default function GridToolbarSearchBox (props) {
  const [formError, setFormError] = React.useState(false)

  const submit = () => {
    if (props.value.length === 0 && props.required) {
      setFormError(true)
    } else {
      props.action()
    }
  }

  return (
        <Fragment>
            <Grid container alignItems="stretch" style={{ display: 'flex' }}>
                <TextField
                    id="outlined-secondary"
                    label="Filter"
                    variant="outlined"
                    color="secondary"
                    defaultValue={props.value}
                    style={{ border: formError ? 'solid 1px red' : '', width: '50%' }}
                    onChange={(e) => props.setValue(e.target.value)}
                /> &nbsp;&nbsp;

                <Button variant="contained" color="secondary" onClick={submit} >Search</Button>
            </Grid>
        </Fragment>
  )
}

GridToolbarSearchBox.propTypes = {
  value: PropTypes.string,
  required: PropTypes.bool,
  action: PropTypes.func,
  setValue: PropTypes.func
}
