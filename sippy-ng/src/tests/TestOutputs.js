import {
  Button,
  CircularProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
} from '@mui/material'
import { styled } from '@mui/material/styles';
import { DirectionsBoat } from '@mui/icons-material'
import { makeStyles } from '@mui/material/styles'
import { safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/lab/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

const PREFIX = 'TestOutputs';

const classes = {
  table: `${PREFIX}-table`
};

// TODO jss-to-styled codemod: The Fragment root was replaced by div. Change the tag if needed.
const Root = styled('div')((
  {
    theme
  }
) => ({
  [`& .${classes.table}`]: {
    minWidth: 650,
    '& .MuiTableCell-root': {
      border: '1px solid #cccccc',
    },
  }
}));

export function TestOutputs(props) {


  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [outputs, setOutputs] = React.useState([])

  useEffect(() => {
    fetchData()
  }, [])

  const fetchData = () => {
    let queryString = ''
    if (props.filterModel && props.filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(props.filterModel))
    }

    fetch(
      process.env.REACT_APP_API_URL +
        `/api/tests/outputs?release=${
          props.release
        }&test=${safeEncodeURIComponent(props.test)}` +
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
          setOutputs(json)
        } else {
          setOutputs([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve test outputs ' + props.release + ', ' + error
        )
      })
  }

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <CircularProgress color="inherit" />
  }

  if (outputs.length === 0) {
    return <Root>No data.</Root>;
  }

  return (
    <Fragment>
      <TableContainer className={classes.table}>
        <Table aria-label="test-outputs">
          <TableBody>
            {outputs.map((v, index) => (
              <TableRow key={`output-${index}`}>
                <TableCell
                  style={{
                    width: '70vw',
                    maxWidth: '70vw',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                  }}
                >
                  <pre style={{ whiteSpace: 'pre-wrap' }}>{v.output}</pre>
                </TableCell>
                <TableCell align="center" style={{ verticalAlign: 'top' }}>
                  <Tooltip title="View in Prow">
                    <Button
                      style={{ justifyContent: 'center' }}
                      target="_blank"
                      startIcon={<DirectionsBoat />}
                      href={v.url}
                    />
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Fragment>
  )
}

TestOutputs.propTypes = {
  release: PropTypes.string.isRequired,
  test: PropTypes.string.isRequired,
  filterModel: PropTypes.object,
}
