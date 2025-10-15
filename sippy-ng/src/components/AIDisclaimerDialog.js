import {
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  FormControlLabel,
} from '@mui/material'
import { CapabilitiesContext } from '../App'
import { useCookies } from 'react-cookie'
import React from 'react'

export default function AIDisclaimerDialog() {
  const capabilities = React.useContext(CapabilitiesContext)
  const [cookies, setCookie] = useCookies(['aiDisclaimerAccepted'])
  const [open, setOpen] = React.useState(false)
  const [dontRemindAI, setDontRemindAI] = React.useState(true)

  React.useEffect(() => {
    if (capabilities.includes('chat') && !cookies['aiDisclaimerAccepted']) {
      setOpen(true)
    }
  }, [capabilities, cookies])

  const handleAccept = () => {
    if (dontRemindAI) {
      const expiryDate = new Date()
      expiryDate.setDate(expiryDate.getDate() + 30)
      setCookie('aiDisclaimerAccepted', 'true', {
        path: '/',
        sameSite: 'Strict',
        expires: expiryDate,
      })
    }
    setOpen(false)
  }

  return (
    <Dialog
      open={open}
      aria-labelledby="ai-disclaimer-dialog-title"
      aria-describedby="ai-disclaimer-dialog-description"
      sx={{ zIndex: 1400 }}
    >
      <DialogContent>
        <DialogContentText id="ai-disclaimer-dialog-description">
          You are about to use a Red Hat tool that utilizes AI technology to
          provide you with relevant information. By proceeding to use the tool,
          you acknowledge that the tool and any output provided are only
          intended for internal use and that information should only be shared
          with those with a legitimate business purpose. Do not include any
          personal information or customer-specific information in your input.
          Responses provided by tools utilizing AI technology should be reviewed
          and verified prior to use.
        </DialogContentText>
      </DialogContent>
      <DialogActions
        sx={{
          justifyContent: 'space-between',
          paddingLeft: 3,
          paddingRight: 3,
        }}
      >
        <FormControlLabel
          control={
            <Checkbox
              checked={dontRemindAI}
              onChange={(e) => setDontRemindAI(e.target.checked)}
              color="primary"
            />
          }
          label="Don't remind me for 30 days"
        />
        <Button
          onClick={handleAccept}
          color="primary"
          variant="contained"
          autoFocus
        >
          OK
        </Button>
      </DialogActions>
    </Dialog>
  )
}
