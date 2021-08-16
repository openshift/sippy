import { createTheme } from '@material-ui/core'

const theme = createTheme()

export const ROW_STYLES = {
  rowSuccess: {
    backgroundColor: theme.palette.success.light,
    color: 'black'
  },
  rowWarning: {
    backgroundColor: theme.palette.warning.light,
    color: 'black'
  },
  rowError: {
    backgroundColor: theme.palette.error.light,
    color: 'black'
  }
}
