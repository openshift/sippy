import { createTheme } from '@material-ui/core/styles'
import { makeStyles } from '@material-ui/styles'
import IconButton from '@material-ui/core/IconButton'
import TextField from '@material-ui/core/TextField'
import {
  GridToolbarDensitySelector,
  GridToolbarFilterButton
} from '@material-ui/data-grid'
import ClearIcon from '@material-ui/icons/Clear'
import SearchIcon from '@material-ui/icons/Search'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import GridToolbarBookmarkMenu from '../datagrid/GridToolbarBookmarkMenu'
import GridToolbarPeriodSelector from '../datagrid/GridToolbarPeriodSelector'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    root: {
      padding: theme.spacing(0.5, 0.5, 0),
      justifyContent: 'space-between',
      display: 'flex',
      alignItems: 'flex-start',
      flexWrap: 'wrap'
    },
    textField: {
      [theme.breakpoints.down('xs')]: {
        width: '100%'
      },
      margin: theme.spacing(1, 0.5, 1.5),
      '& .MuiSvgIcon-root': {
        marginRight: theme.spacing(0.5)
      },
      '& .MuiInput-underline:before': {
        borderBottom: `1px solid ${theme.palette.divider}`
      }
    }
  }),
  { defaultTheme }
)

export default function GridToolbar (props) {
  const classes = useStyles()

  const [search, setSearch] = React.useState('')

  return (
        <div className={classes.root}>
            <div>
                <GridToolbarFilterButton />
                <GridToolbarBookmarkMenu bookmarks={props.bookmarks} setFilterModel={props.setFilterModel} />
                <GridToolbarPeriodSelector selectPeriod={props.selectPeriod} period={props.period} />
                <GridToolbarDensitySelector />

            </div>
            <div alignItems="center">
                <TextField
                    variant="standard"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    onBlur={() => props.doSearch(search)}
                    placeholder="Search…"
                    InputProps={{
                      endAdornment: (
                            <Fragment>
                                <IconButton
                                    title="Search"
                                    aria-label="Search"
                                    size="small"
                                    onClick={() => props.doSearch(search)}
                                >
                                    <SearchIcon fontSize="small" />
                                </IconButton>
                                <IconButton
                                    title="Clear"
                                    aria-label="Clear"
                                    size="small"
                                    onClick={() => { props.clearSearch(); setSearch('') }}
                                >
                                    <ClearIcon fontSize="small" />
                                </IconButton>
                            </Fragment>
                      )
                    }}
                />
            </div>
        </div>
  )
}

GridToolbar.propTypes = {
  bookmarks: PropTypes.object.isRequired,
  clearSearch: PropTypes.func.isRequired,
  doSearch: PropTypes.func.isRequired,
  onChange: PropTypes.func.isRequired,
  period: PropTypes.string,
  selectPeriod: PropTypes.func.isRequired,
  setFilterModel: PropTypes.func.isRequired,
  value: PropTypes.string
}
