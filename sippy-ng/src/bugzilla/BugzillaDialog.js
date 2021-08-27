import { Button, Divider, Tooltip, Typography } from '@material-ui/core'
import { Close, Info } from '@material-ui/icons'
import BugTable from './BugTable'
import bugzillaURL from './BugzillaUtils'
import Dialog from '@material-ui/core/Dialog'
import DialogContent from '@material-ui/core/DialogContent'
import DialogTitle from '@material-ui/core/DialogTitle'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export const LINKED_BUGS = `Linked bugs are bugs that mention the failing test name
and are targeted to the release being reported on.`

export const ASSOCIATED_BUGS = `Associated bugs are bugs that mention the failing test
name but are not targeted to the release being reported on.`

/**
 * BugzillaDialog shows the bugs both linked and associated with a
 * job or test. It also has a link to prefill in a new bug.
 */
export default function BugzillaDialog(props) {
  return (
    <Fragment>
      <Dialog
        scroll="paper"
        style={{ verticalAlign: 'top' }}
        maxWidth="md"
        fullWidth={true}
        open={props.isOpen}
        onClose={props.close}
        aria-labelledby="form-dialog-title"
      >
        <DialogTitle id="form-dialog-title" style={{ textAlign: 'right' }}>
          <Button startIcon={<Close />} onClick={props.close} />
        </DialogTitle>
        <DialogContent>
          <Typography variant="h5">{props.item.name}</Typography>
          <Divider />

          <Typography variant="h6" style={{ margin: 20 }}>
            Linked Bugs
            <Tooltip title={LINKED_BUGS}>
              <Info />
            </Tooltip>
          </Typography>

          <BugTable bugs={props.item.bugs} />

          <Typography variant="h6" style={{ margin: 20 }}>
            Associated Bugs
            <Tooltip title={ASSOCIATED_BUGS}>
              <Info />
            </Tooltip>
          </Typography>

          <BugTable bugs={props.item.associated_bugs} />

          <Button
            target="_blank"
            href={bugzillaURL(props.release, props.item)}
            variant="contained"
            color="primary"
            style={{ marginTop: 20 }}
          >
            Open a new bug
          </Button>
        </DialogContent>
      </Dialog>
    </Fragment>
  )
}

BugzillaDialog.defaultProps = {
  item: {
    name: '',
    bugs: [],
    associated_bugs: [],
  },
  classes: {},
}

BugzillaDialog.propTypes = {
  release: PropTypes.string,
  item: PropTypes.object.isRequired,
  classes: PropTypes.object,
  isOpen: PropTypes.bool,
  close: PropTypes.func,
}
