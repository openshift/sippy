import { Box } from '@mui/material'
import { styled } from '@mui/material/styles'
import InfoIcon from '@mui/icons-material/Info'

export const StyledInfoIcon = styled(InfoIcon)(({ theme }) => ({
  marginLeft: theme.spacing(1),
  fontSize: 20,
  opacity: 0.6,
}))

export const CardHeaderRow = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  marginBottom: theme.spacing(2),
}))
