import { createTheme } from '@material-ui/core'
import { scale } from 'chroma-js'

const theme = createTheme()

export function generateClasses(threshold) {
  const colors = scale([
    theme.palette.error.light,
    theme.palette.warning.light,
    theme.palette.success.light,
  ]).domain([threshold.error, threshold.warning, threshold.success])

  const classes = {}

  for (let i = 0; i <= 100; i++) {
    classes['row-percent-' + i] = { backgroundColor: colors(i).hex() }
  }

  return classes
}

export const ROW_STYLES = {
  rowSuccess: {
    backgroundColor: theme.palette.success.light,
    color: 'black',
  },
  rowWarning: {
    backgroundColor: theme.palette.warning.light,
    color: 'black',
  },
  rowError: {
    backgroundColor: theme.palette.error.light,
    color: 'black',
  },
}
