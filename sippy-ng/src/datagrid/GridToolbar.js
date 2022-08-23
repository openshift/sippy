import { createTheme } from '@material-ui/core/styles'
import { GridToolbarDensitySelector } from '@material-ui/data-grid'
import { makeStyles } from '@material-ui/styles'
import ClearIcon from '@material-ui/icons/Clear'
import GridToolbarBookmarkMenu from '../datagrid/GridToolbarBookmarkMenu'
import GridToolbarDownload from './GridToolbarDownload'
import GridToolbarFilterMenu from './GridToolbarFilterMenu'
import GridToolbarPeriodSelector from '../datagrid/GridToolbarPeriodSelector'
import GridToolbarViewSelector from './GridToolbarViewSelector'
import IconButton from '@material-ui/core/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SearchIcon from '@material-ui/icons/Search'
import TextField from '@material-ui/core/TextField'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    root: {
      padding: theme.spacing(0.5, 0.5, 0),
      justifyContent: 'space-between',
      display: 'flex',
      alignItems: 'flex-start',
      flexWrap: 'wrap',
    },
    textField: {
      [theme.breakpoints.down('xs')]: {
        width: '100%',
      },
      margin: theme.spacing(1, 0.5, 1.5),
      '& .MuiSvgIcon-root': {
        marginRight: theme.spacing(0.5),
      },
      '& .MuiInput-underline:before': {
        borderBottom: `1px solid ${theme.palette.divider}`,
      },
    },
  }),
  { defaultTheme }
)

export default function GridToolbar(props) {
  const classes = useStyles()

  const [search, setSearch] = React.useState('')

  return (
    <div className={classes.root}>
      <div>
        <GridToolbarFilterMenu
          columns={props.columns}
          filterModel={props.filterModel}
          setFilterModel={props.setFilterModel}
        />

        {props.bookmarks ? (
          <GridToolbarBookmarkMenu
            bookmarks={props.bookmarks}
            setFilterModel={props.addFilters}
          />
        ) : (
          ''
        )}
        {props.period ? (
          <GridToolbarPeriodSelector
            selectPeriod={props.selectPeriod}
            period={props.period}
          />
        ) : (
          ''
        )}
        {props.views && props.views.length > 1 ? (
          <GridToolbarViewSelector
            setView={props.selectView}
            views={props.views}
            view={props.view}
          />
        ) : (
          ''
        )}
        <GridToolbarDensitySelector />
        {props.downloadDataFunc ? (
          <GridToolbarDownload
            getData={props.downloadDataFunc}
            filePrefix={props.downloadFilePrefix}
          />
        ) : (
          ''
        )}
      </div>
      {props.doSearch ? (
        <div>
          <TextField
            variant="standard"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && props.doSearch(search)}
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
                    onClick={() => {
                      props.clearSearch()
                      setSearch('')
                    }}
                  >
                    <ClearIcon fontSize="small" />
                  </IconButton>
                </Fragment>
              ),
            }}
          />
        </div>
      ) : (
        ''
      )}
    </div>
  )
}

GridToolbar.propTypes = {
  bookmarks: PropTypes.array,
  clearSearch: PropTypes.func.isRequired,
  doSearch: PropTypes.func.isRequired,
  period: PropTypes.string,
  selectPeriod: PropTypes.func,
  columns: PropTypes.array,
  view: PropTypes.string,
  views: PropTypes.object,
  selectView: PropTypes.func,
  filterModel: PropTypes.object,
  setFilterModel: PropTypes.func.isRequired,
  addFilters: PropTypes.func.isRequired,
  value: PropTypes.string,
  downloadDataFunc: PropTypes.func,
  downloadFilePrefix: PropTypes.string,
}
