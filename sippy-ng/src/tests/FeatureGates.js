import { Container, Typography } from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { Link, useNavigate } from 'react-router-dom'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import {
  pathForTestSubstringByVariant,
  safeEncodeURIComponent,
  SafeJSONParam,
} from '../helpers'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

/**
 * Feature gates is the landing page for feature gates.
 */
export default function FeatureGates(props) {
  const navigate = useNavigate()

  const { classes } = props
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const defaultFilterModel = React.useMemo(() => {
    if (!props.release) return { items: [] }

    const [major, minor] = props.release.split('.').map(Number)

    return {
      items: [
        {
          columnField: 'enabled',
          not: true,
          operatorValue: 'has entry containing',
          value: 'Default:Hypershift',
        },
        {
          columnField: 'enabled',
          not: true,
          operatorValue: 'has entry containing',
          value: 'Default:SelfManagedHA',
        },
        {
          columnField: 'first_seen_in_major',
          operatorValue: '=',
          value: String(major),
        },
        {
          columnField: 'first_seen_in_minor',
          operatorValue: '>=',
          value: String(Math.max(minor - 2, 15)), // don't go below 4.15
        },
      ],
    }
  }, [props.release])

  const staleFeatureGates = React.useMemo(() => {
    if (!props.release) return []

    const [major, minor] = props.release.split('.').map(Number)

    return [
      {
        columnField: 'enabled',
        not: true,
        operatorValue: 'has entry',
        value: 'Default:Hypershift',
      },
      {
        columnField: 'enabled',
        not: true,
        operatorValue: 'has entry',
        value: 'Default:SelfManagedHA',
      },
      {
        columnField: 'first_seen_in_major',
        operatorValue: '=',
        value: String(major),
      },
      {
        columnField: 'first_seen_in_minor',
        operatorValue: '<=',
        value: String(Math.max(minor - 2, 15)),
      },
    ]
  }, [props.release])

  const [filterModel = defaultFilterModel, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  const bookmarks = [
    {
      name: 'Recent techpreview/devpreview features',
      model: defaultFilterModel.items,
    },
    {
      name: 'Stale features (unpromoted in > 2 releases)',
      model: staleFeatureGates,
    },
    {
      name: 'Default:Hypershift',
      model: [
        {
          columnField: 'enabled',
          operatorValue: 'has entry',
          value: 'Default:Hypershift',
        },
      ],
    },
    {
      name: 'Default:SelfManagedHA',
      model: [
        {
          columnField: 'enabled',
          operatorValue: 'has entry',
          value: 'Default:SelfManagedHA',
        },
      ],
    },
    {
      name: 'TechPreview:SelfManagedHA',
      model: [
        {
          columnField: 'enabled',
          operatorValue: 'has entry',
          value: 'TechPreviewNoUpgrade:SelfManagedHA',
        },
        {
          columnField: 'enabled',
          not: true,
          operatorValue: 'has entry',
          value: 'Default:SelfManagedHA',
        },
      ],
    },
    {
      name: 'TechPreview:Hypershift',
      model: [
        {
          columnField: 'enabled',
          operatorValue: 'has entry',
          value: 'TechPreviewNoUpgrade:Hypershift',
        },
        {
          columnField: 'enabled',
          not: true,
          operatorValue: 'has entry',
          value: 'Default:Hypershift',
        },
      ],
    },
  ]

  const [sortField = props.sortField, setSortField] = useQueryParam(
    'sortField',
    StringParam
  )

  const [pageSize = props.pageSize, setPageSize] = useQueryParam(
    'pageSize',
    NumberParam
  )

  const [sort = props.sort, setSort] = useQueryParam('sort', StringParam)

  const updateSortModel = (model) => {
    if (model.length === 0) {
      return
    }

    if (sort !== model[0].sort) {
      setSort(model[0].sort)
    }

    if (sortField !== model[0].field) {
      setSortField(model[0].field)
    }
  }

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'feature_gate'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'feature_gate',
      operatorValue: 'contains',
      value: searchValue,
    })
    setFilterModel(currentFilters)
  }

  const addFilters = (filter) => {
    const currentFilters = filterModel.items.filter((item) => {
      for (let i = 0; i < filter.length; i++) {
        if (filter[i].columnField === item.columnField) {
          return false
        }
      }

      return item.value !== ''
    })

    filter.forEach((item) => {
      if (item.value && item.value !== '') {
        currentFilters.push(item)
      }
    })
    setFilterModel({
      items: currentFilters,
      linkOperator: filterModel.linkOperator || 'and',
    })
  }

  const columns = [
    {
      field: 'id',
      filterable: false,
      hide: true,
    },
    { field: 'feature_gate', headerName: 'Feature Gate', flex: 3 },
    {
      field: 'unique_test_count',
      headerName: 'Test Count',
      type: 'number',
      flex: 2,
      renderCell: (params) => {
        return <Link to={linkForFGTests(params)}>{params.value}</Link>
      },
    },
    {
      field: 'enabled',
      headerName: 'Enabled',
      type: 'array',
      flex: 4,
      renderCell: (params) => (
        <div style={{ whiteSpace: 'pre' }}>
          {params.value ? params.value.join('\n') : ''}
        </div>
      ),
    },
    {
      field: 'first_seen_in',
      headerName: 'First seen in',
      type: 'string',
      flex: 2,
    },
    {
      field: 'first_seen_in_major',
      headerName: 'First seen in major',
      type: 'number',
      hide: true,
    },
    {
      field: 'first_seen_in_minor',
      headerName: 'First seen in minor',
      type: 'number',
      hide: true,
    },
  ]

  const linkForFGTests = (params) => {
    let fgAnnotation = `FeatureGate:${params.row.feature_gate}]`
    if (params.row.feature_gate.includes('Install')) {
      fgAnnotation = 'install should succeed'
    }
    return pathForTestSubstringByVariant(props.release, fgAnnotation)
  }

  const fetchData = () => {
    let queryString = ''
    if (filterModel && filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(filterModel))
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/feature_gates?release=' +
        props.release +
        queryString
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        if (json != null) {
          setRows(json)
        } else {
          setRows([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve feature gates ' + props.release + ', ' + error
        )
      })
  }

  const onRowClick = (params) => {
    console.log('clicked')
    navigate(linkForFGTests(params))
  }

  useEffect(() => {
    setLoaded(false)
  }, [filterModel])

  useEffect(() => {
    fetchData()
    document.title = `Sippy > ${props.release} > Feature Gates`
  }, [filterModel, sort, sortField])

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Feature Gates" />
      <Container size="xl">
        <Typography align="center" variant="h4" sx={{ marginBottom: 2 }}>
          Feature Gates for {props.release}
        </Typography>
        <Alert severity="info" sx={{ mb: 2 }}>
          Click on a row to view tests by variant for that feature gate. Note,
          we only count tests that have had runs in the last 7 days.
        </Alert>
        {fetchError && (
          <Typography color="error" align="center">
            {fetchError}
          </Typography>
        )}
        {isLoaded ? (
          <DataGrid
            components={{ Toolbar: GridToolbar }}
            rows={rows}
            columns={columns}
            getRowHeight={() => 'auto'}
            autoHeight={true}
            rowsPerPageOptions={[10, 25, 50]}
            sortModel={[
              {
                field: sortField,
                sort: sort,
              },
            ]}
            pageSize={pageSize}
            onPageSizeChange={(newPageSize) => setPageSize(newPageSize)}
            sortingOrder={['desc', 'asc']}
            filterMode="server"
            sortingMode="server"
            onSortModelChange={(m) => updateSortModel(m)}
            sx={{
              '& .MuiDataGrid-row:hover': {
                cursor: 'pointer', // Change cursor on hover
              },
            }}
            disableSelectionOnClick
            filterModel={filterModel}
            onRowClick={onRowClick}
            componentsProps={{
              toolbar: {
                bookmarks: bookmarks,
                columns: columns,
                clearSearch: () => requestSearch(''),
                doSearch: requestSearch,
                addFilters: (m) => addFilters(m),
                filterModel: filterModel,
                setFilterModel: setFilterModel,
                downloadDataFunc: () => {
                  return rows
                },
                downloadFilePrefix: 'feature-gates',
              },
            }}
          />
        ) : (
          <Typography align="center">Loading...</Typography>
        )}
      </Container>
    </Fragment>
  )
}

FeatureGates.defaultProps = {
  pageSize: 25,
  rowsPerPageOptions: [5, 10, 25, 50, 100],
  sortField: 'unique_test_count',
  sort: 'asc',
}

FeatureGates.propTypes = {
  classes: PropTypes.object,
  release: PropTypes.string.isRequired,
  pageSize: PropTypes.number,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  rowsPerPageOptions: PropTypes.array,
}
