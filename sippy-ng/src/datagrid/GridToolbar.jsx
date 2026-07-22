import { GridToolbarDensitySelector } from '@mui/x-data-grid'
import { makeStyles, useTheme } from '@mui/styles'
import ClearIcon from '@mui/icons-material/Clear'
import GridToolbarBookmarkMenu from '../datagrid/GridToolbarBookmarkMenu'
import GridToolbarDownload from './GridToolbarDownload'
import GridToolbarFilterMenu from './GridToolbarFilterMenu'
import GridToolbarPeriodSelector from '../datagrid/GridToolbarPeriodSelector'
import GridToolbarViewSelector from './GridToolbarViewSelector'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SearchIcon from '@mui/icons-material/Search'
import TextField from '@mui/material/TextField'

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(0.5, 0.5, 0),
    justifyContent: 'space-between',
    display: 'flex',
    alignItems: 'flex-start',
    flexWrap: 'wrap',
  },
  textField: {
    [theme.breakpoints.down('sm')]: {
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
}))

export default function GridToolbar(props) {
  const theme = useTheme()
  const classes = useStyles(theme)

  const [search, setSearch] = React.useState('')

  return (
    <div className={classes.root}>
      <div>
        <GridToolbarFilterMenu
          columns={props.columns}
          filterModel={props.filterModel}
          setFilterModel={props.setFilterModel}
          autocompleteData={props.autocompleteData}
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
        {props.views && Object.keys(props.views).length > 1 ? (
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
  autocompleteData: PropTypes.array,
}
