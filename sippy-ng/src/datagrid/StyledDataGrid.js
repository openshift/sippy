import { DataGrid } from '@mui/x-data-grid'
import { styled } from '@mui/material/styles'

export const StyledDataGrid = styled(DataGrid)(({ theme }) => ({
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
}))
