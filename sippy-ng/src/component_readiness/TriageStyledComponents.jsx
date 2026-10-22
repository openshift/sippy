import { Box, DialogContent, IconButton, Tabs } from '@mui/material'
import { styled } from '@mui/material/styles'

export const TriageTabs = styled(Tabs)(({ theme }) => ({
  borderBottom: `1px solid ${theme.palette.divider}`,
  '& .MuiTab-root': {
    border: `1px solid ${theme.palette.divider}`,
    borderBottom: 'none',
    margin: '0 2px',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
    '&.Mui-selected': {
      backgroundColor: theme.palette.background.paper,
      borderColor: theme.palette.primary.main,
    },
  },
}))

export const TriageDialogContent = styled(DialogContent)({
  overflow: 'auto',
  flex: 1,
  paddingTop: '8px',
  paddingBottom: '8px',
})

export const LoadingContainer = styled(Box)(({ theme }) => ({
  alignItems: 'center',
  display: 'flex',
  flexDirection: 'column',
  gap: theme.spacing(2),
  justifyContent: 'center',
  minHeight: '400px',
}))

export const CloseButton = styled(IconButton)({
  position: 'absolute',
  right: 8,
  top: 8,
})

export const CompactLoadingBox = styled(Box)({
  padding: 24,
  display: 'flex',
  justifyContent: 'center',
  alignItems: 'center',
})
