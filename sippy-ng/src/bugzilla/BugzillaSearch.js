import PropTypes from 'prop-types'
import Button from '@material-ui/core/Button'
import Dialog from '@material-ui/core/Dialog'
import DialogActions from '@material-ui/core/DialogActions'
import DialogContent from '@material-ui/core/DialogContent'
import DialogContentText from '@material-ui/core/DialogContentText'
import DialogTitle from '@material-ui/core/DialogTitle'
import TextField from '@material-ui/core/TextField'
import React, { Fragment } from 'react'

/**
 * BugzillaSearch is a dialog for searching the OpenShift Container Platform
 * product on Red Hat Bugzilla.
 */
export default function BugzillaSearch (props) {
  const [query, setQuery] = React.useState('')

  const bugzillaDialogQuery = (f) => {
    setQuery(f.target.value)
  }

  const handleBugzillaQuery = (f) => {
    window.open('https://bugzilla.redhat.com/buglist.cgi?query_format=specific&order=Importance&no_redirect=1&bug_status=__open__&product=OpenShift+Container+Platform&content=' + encodeURIComponent(query))
    props.close()
  }

  return (
        <Fragment>
            <Dialog open={props.isOpen} onClose={props.close} aria-labelledby="form-dialog-title">
                <DialogTitle id="form-dialog-title">Search Bugzilla</DialogTitle>
                <DialogContent>
                    <DialogContentText>
                        Search the OpenShift Bugzilla
                    </DialogContentText>
                    <TextField
                        autoFocus
                        margin="dense"
                        id="name"
                        label="Query"
                        type="text"
                        fullWidth
                        onChange={bugzillaDialogQuery}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={props.close} color="primary">
                        Cancel
                    </Button>
                    <Button onClick={handleBugzillaQuery} color="primary">
                        Search
                    </Button>
                </DialogActions>
            </Dialog>
        </Fragment>
  )
}

BugzillaSearch.propTypes = {
  open: PropTypes.func.isRequired,
  close: PropTypes.func.isRequired,
  isOpen: PropTypes.bool.isRequired
}
