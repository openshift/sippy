import { Box } from '@mui/material'
import { GridToolbarDensitySelector } from '@mui/x-data-grid'
import { styled } from '@mui/material/styles'
import ClearIcon from '@mui/icons-material/Clear'
import GridToolbarBookmarkMenu from '../datagrid/GridToolbarBookmarkMenu'
import GridToolbarDownload from './GridToolbarDownload'
import GridToolbarFilterMenu from './GridToolbarFilterMenu'
import GridToolbarPeriodSelector from '../datagrid/GridToolbarPeriodSelector'
import GridToolbarViewSelector from './GridToolbarViewSelector'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SearchIcon from '@mui/icons-material/Search'
import TextField from '@mui/material/TextField'

const ToolbarContainer = styled(Box)(({ theme }) => ({
  padding: theme.spacing(0.5, 0.5, 0),
  justifyContent: 'space-between',
  display: 'flex',
  alignItems: 'flex-start',
  flexWrap: 'wrap',
}))

export default function GridToolbar(props) {
  const [search, setSearch] = React.useState('')

  const filterSearchValue = (() => {
    if (!props.searchField || !props.filterModel?.items) return undefined
    const filtersForField = props.filterModel.items.filter(
      (f) => f.field === props.searchField
    )
    if (filtersForField.length !== 1) return undefined
    const filter = filtersForField[0]
    if (filter.operator === 'contains' && filter.not !== true && filter.value) {
      return filter.value
    }
    return undefined
  })()

  useEffect(() => {
    setSearch(filterSearchValue ?? '')
  }, [filterSearchValue])

  return (
    <ToolbarContainer>
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
    </ToolbarContainer>
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
  searchField: PropTypes.string,
}
