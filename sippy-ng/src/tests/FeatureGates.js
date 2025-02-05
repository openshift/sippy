import { Container, Typography } from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { Link, useHistory } from 'react-router-dom'
import { pathForTestSubstringByVariant, SafeJSONParam } from '../helpers'
import { useQueryParam, withDefault } from 'use-query-params'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

/**
 * Feature gates is the landing page for feature gates.
 */
export default function FeatureGates(props) {
  const history = useHistory()

  const { classes } = props
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const [filterModel, setFilterModel] = useQueryParam(
    'filterModel',
    withDefault(SafeJSONParam, { items: [] })
  )

  const [sortModel, setSortModel] = React.useState([
    { field: 'unique_test_count', sort: 'asc' }, // Default sort
  ])

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'feature_gate'
    )
    currentFilters.items.push({
      columnField: 'feature_gate',
      operatorValue: 'contains',
      value: searchValue,
    })
    setFilterModel(currentFilters)
  }

  const columns = [
    {
      field: 'id',
      hide: true,
    },
    {
      field: 'type',
      headerName: 'Type',
      width: 150,
      renderCell: (params) =>
        params.value === 'OCPFeatureGate' ? 'OpenShift' : 'Kubernetes',
    },
    { field: 'feature_gate', headerName: 'Feature Gate', width: 300 },
    {
      field: 'unique_test_count',
      headerName: 'Unique Test Count',
      type: 'number',
      width: 200,
      renderCell: (params) => {
        return <Link to={linkForFGTests(params)}>{params.value}</Link>
      },
    },
  ]

  const linkForFGTests = (params) => {
    const fgAnnotation = `[${params.row.type}:${params.row.feature_gate}]`
    return pathForTestSubstringByVariant(props.release, fgAnnotation)
  }

  const fetchData = () => {
    fetch(
      process.env.REACT_APP_API_URL +
        '/api/feature_gates?release=' +
        props.release
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
          'Could not retrieve tests ' + props.release + ', ' + error
        )
      })
  }

  const onRowClick = (params) => {
    console.log('clicked')
    history.push(linkForFGTests(params))
  }

  useEffect(() => {
    fetchData()
    document.title = `Sippy > ${props.release} > Feature Gates`
  }, [])

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Feature Gates" />
      <Container size="xl">
        <Typography align="center" variant="h4" sx={{ marginBottom: 2 }}>
          Feature Gates for {props.release}
        </Typography>
        <Alert severity="info" sx={{ mb: 2 }}>
          Click on a row to view tests for that feature gate. Row color is based
          on number of tests only; a feature gate should have at least 5 tests.
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
            pageSize={25}
            autoHeight={true}
            rowsPerPageOptions={[10, 25, 50]}
            sortModel={sortModel} // Controlled sortModel
            onSortModelChange={(newModel) => setSortModel(newModel)}
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
                columns: columns,
                filterModel: filterModel,
                setFilterModel: setFilterModel,
                clearSearch: () => requestSearch(''),
                doSearch: requestSearch,
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

FeatureGates.propTypes = {
  classes: PropTypes.object,
  release: PropTypes.string.isRequired,
}
