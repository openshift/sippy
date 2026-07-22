import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  Snackbar,
  TextField,
  Typography,
} from '@mui/material'
import {
  Close as CloseIcon,
  ContentCopy as ContentCopyIcon,
} from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import { useShareActions, useShareState } from './store/useChatStore'
import React from 'react'

const useStyles = makeStyles((theme) => ({
  shareDialogTitle: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingRight: theme.spacing(1),
  },
  shareDialogContent: {
    paddingTop: theme.spacing(2),
  },
  shareTextField: {
    marginTop: theme.spacing(2),
    '& .MuiOutlinedInput-root': {
      fontFamily: 'monospace',
      fontSize: '0.9rem',
    },
  },
  copyButton: {
    marginRight: theme.spacing(1),
  },
}))

/**
 * Dialog for displaying and copying a shared conversation link
 */
export default function ShareDialog() {
  const classes = useStyles()

  const { shareDialogOpen, sharedUrl, shareSnackbar } = useShareState()
  const { setShareDialogOpen, closeShareSnackbar, copyToClipboard } =
    useShareActions()

  return (
    <>
      <Dialog
        open={shareDialogOpen}
        onClose={() => setShareDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle className={classes.shareDialogTitle}>
          <Typography variant="h6" component="span">
            Conversation Shared
          </Typography>
          <IconButton
            edge="end"
            onClick={() => setShareDialogOpen(false)}
            aria-label="close"
            size="small"
          >
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent className={classes.shareDialogContent}>
          <DialogContentText>
            Your conversation has been shared! Anyone with this link can view
            and continue the conversation.
          </DialogContentText>
          <DialogContentText sx={{ mt: 1, fontStyle: 'italic' }}>
            Note: Shared conversations will be available for up to 90 days.
          </DialogContentText>
          <TextField
            autoFocus
            margin="dense"
            label="Shareable Link"
            fullWidth
            variant="outlined"
            value={sharedUrl}
            InputProps={{
              readOnly: true,
            }}
            className={classes.shareTextField}
            onClick={(e) => e.target.select()}
          />
        </DialogContent>
        <DialogActions>
          <Button
            onClick={copyToClipboard}
            startIcon={<ContentCopyIcon />}
            variant="contained"
            className={classes.copyButton}
          >
            Copy Link
          </Button>
          <Button onClick={() => setShareDialogOpen(false)} variant="outlined">
            Close
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={shareSnackbar.open}
        autoHideDuration={6000}
        onClose={closeShareSnackbar}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={closeShareSnackbar}
          severity={shareSnackbar.severity}
          sx={{ width: '100%' }}
        >
          {shareSnackbar.message}
        </Alert>
      </Snackbar>
    </>
  )
}

ShareDialog.propTypes = {}
