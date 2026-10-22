import { styled } from '@mui/material/styles'
import InfoIcon from '@mui/icons-material/Info'

export const StyledInfoIcon = styled(InfoIcon)(({ theme }) => ({
  marginLeft: theme.spacing(1),
  fontSize: 20,
  opacity: 0.6,
}))
