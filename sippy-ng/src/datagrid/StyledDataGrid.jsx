import { DataGrid } from '@mui/x-data-grid'
import { styled } from '@mui/material/styles'
import chroma from 'chroma-js'

export const StyledDataGrid = styled(DataGrid)({
  '& .wrapHeader .MuiDataGrid-columnHeaderTitle': {
    textOverflow: 'ellipsis',
    display: '-webkit-box',
    WebkitLineClamp: 2,
    WebkitBoxOrient: 'vertical',
    overflow: 'hidden',
    overflowWrap: 'break-word',
    lineHeight: '20px',
    whiteSpace: 'normal',
  },
})

export function createColoredDataGrid(thresholds, invert = false) {
  return styled(StyledDataGrid)(({ theme }) => {
    let scaleColors = [
      theme.palette.error.light,
      theme.palette.warning.light,
      theme.palette.success.light,
    ]
    if (invert) {
      scaleColors = [
        theme.palette.success.light,
        theme.palette.warning.light,
        theme.palette.error.light,
      ]
    }
    const colors = chroma
      .scale(scaleColors)
      .domain([thresholds.error, thresholds.warning, thresholds.success])

    const styles = {}
    for (let i = 0; i <= 100; i++) {
      styles[`& .row-percent-${i}`] = { backgroundColor: colors(i).hex() }
    }
    styles['& .overall'] = {
      filter: 'brightness(80%) contrast(150%)',
      borderBottom: '5px solid black',
    }
    return styles
  })
}
