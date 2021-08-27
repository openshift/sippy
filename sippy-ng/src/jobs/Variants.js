import { Alert } from '@material-ui/lab'
import { makeStyles, useTheme } from '@material-ui/core/styles'
import { PropTypes } from 'prop-types'
import { VARIANT_THRESHOLDS } from '../constants'
import Collapse from '@material-ui/core/Collapse'
import IconButton from '@material-ui/core/IconButton'
import JobTable from './JobTable'
import KeyboardArrowDownIcon from '@material-ui/icons/KeyboardArrowDown'
import KeyboardArrowUpIcon from '@material-ui/icons/KeyboardArrowUp'
import Paper from '@material-ui/core/Paper'
import PassRateIcon from '../components/PassRateIcon'
import React, { Fragment, useEffect } from 'react'
import Table from '@material-ui/core/Table'
import TableBody from '@material-ui/core/TableBody'
import TableCell from '@material-ui/core/TableCell'
import TableContainer from '@material-ui/core/TableContainer'
import TableHead from '@material-ui/core/TableHead'
import TableRow from '@material-ui/core/TableRow'

const useRowStyles = makeStyles({
  root: {
    '& > *': {
      borderBottom: 'unset',
      color: 'black',
    },
  },
})

function Row(props) {
  const { row } = props
  const [open, setOpen] = React.useState(false)
  const classes = useRowStyles()

  return (
    <Fragment>
      <TableRow
        className={classes.root}
        style={{ backgroundColor: props.bgColor }}
      >
        <TableCell>
          <IconButton
            style={{ color: 'black' }}
            aria-label="expand row"
            size="small"
            onClick={() => setOpen(!open)}
          >
            {open ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
          </IconButton>
        </TableCell>
        <TableCell component="th" scope="row">
          {row.platform}
        </TableCell>
        <TableCell align="left">
          {row.passRates.latest.percentage.toFixed(2)}% (
          {row.passRates.latest.runs} runs)
        </TableCell>
        <TableCell align="center">
          <PassRateIcon
            improvement={
              row.passRates.latest.percentage - row.passRates.prev.percentage
            }
          />
        </TableCell>
        <TableCell align="left">
          {row.passRates.prev.percentage.toFixed(2)}% ({row.passRates.prev.runs}{' '}
          runs)
        </TableCell>
      </TableRow>
      <TableRow>
        <TableCell style={{ paddingBottom: 0, paddingTop: 0 }} colSpan={6}>
          <Collapse in={open} timeout="auto" unmountOnExit>
            <JobTable
              briefTable={true}
              hideControls={true}
              filterModel={{
                items: [
                  {
                    id: 99,
                    columnField: 'variants',
                    operatorValue: 'contains',
                    value: row.platform,
                  },
                ],
              }}
              release={props.release}
            />
          </Collapse>
        </TableCell>
      </TableRow>
    </Fragment>
  )
}

Row.propTypes = {
  row: PropTypes.object.isRequired,
  release: PropTypes.string.isRequired,
  bgColor: PropTypes.string,
}

export default function Variants(props) {
  const theme = useTheme()

  const [jobs, setJobs] = React.useState([])
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')

  const rowBackground = (percent) => {
    if (percent >= VARIANT_THRESHOLDS.success) {
      return theme.palette.success.light
    } else if (percent >= VARIANT_THRESHOLDS.warning) {
      return theme.palette.warning.light
    } else if (percent >= VARIANT_THRESHOLDS.error) {
      return theme.palette.error.light
    }
  }

  const fetchData = () => {
    fetch(process.env.REACT_APP_API_URL + '/json?release=' + props.release)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setJobs(json[props.release].jobPassRateByVariant)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve release ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <TableContainer component={Paper}>
      <Table aria-label="collapsible table">
        <TableHead>
          <TableRow>
            <TableCell />
            <TableCell>Variant</TableCell>
            <TableCell>Current Period</TableCell>
            <TableCell></TableCell>
            <TableCell>Previous Period</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {jobs.map((row) => (
            <Row
              key={row.platform}
              bgColor={rowBackground(row.passRates.latest.percentage)}
              row={row}
              release={props.release}
            />
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

Variants.propTypes = {
  release: PropTypes.string.isRequired,
}
