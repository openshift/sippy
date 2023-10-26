import './TestByVariantTable.css'
import { Link } from 'react-router-dom'
import { pathForExactTestAnalysis } from '../helpers'
import { scale } from 'chroma-js'
import { TableContainer, Tooltip, Typography } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import Cookies from 'universal-cookie'
import FormControlLabel from '@material-ui/core/FormControlLabel'
import FormGroup from '@material-ui/core/FormGroup'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import Switch from '@material-ui/core/Switch'
import Table from '@material-ui/core/Table'
import TableBody from '@material-ui/core/TableBody'
import TableCell from '@material-ui/core/TableCell'
import TableHead from '@material-ui/core/TableHead'
import TableRow from '@material-ui/core/TableRow'

function PassRateCompare(props) {
  const { previous, current } = props

  return (
    <Fragment>
      {current.toFixed(2)}%
      <PassRateIcon improvement={current - previous} />
      {previous.toFixed(2)}%
    </Fragment>
  )
}

PassRateCompare.propTypes = {
  previous: PropTypes.number,
  current: PropTypes.number,
}

function Cell(props) {
  const { result } = props
  const theme = useTheme()

  const cellBackground = (percent) => {
    const colorScale = scale([
      theme.palette.error.light,
      theme.palette.warning.light,
      theme.palette.success.light,
    ]).domain(props.colorScale)
    return colorScale(percent).hex()
  }

  if (result === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          className="cell-result"
          style={{
            textAlign: 'center',
            backgroundColor: theme.palette.text.disabled,
          }}
        >
          <HelpOutlineIcon style={{ color: theme.palette.text.disabled }} />
        </TableCell>
      </Tooltip>
    )
  } else if (result.current_runs === 0) {
    return (
      <Tooltip title="No runs in the current period">
        <TableCell
          className="cell-result"
          style={{
            textAlign: 'center',
            backgroundColor: theme.palette.text.disabled,
          }}
        >
          <HelpOutlineIcon style={{ color: theme.palette.text.disabled }} />
        </TableCell>
      </Tooltip>
    )
  } else if (props.showFull) {
    return (
      <TableCell
        className="cell-result"
        style={{
          textAlign: 'center',
          backgroundColor: cellBackground(result.current_pass_percentage),
        }}
      >
        <PassRateCompare
          current={result.current_pass_percentage}
          previous={result.previous_pass_percentage}
        />
      </TableCell>
    )
  } else {
    return (
      <Tooltip
        title={
          <PassRateCompare
            current={result.current_pass_percentage}
            previous={result.previous_pass_percentage}
          />
        }
      >
        <TableCell
          className="cell-result"
          style={{
            textAlign: 'center',
            backgroundColor: cellBackground(result.current_pass_percentage),
          }}
        >
          <PassRateIcon
            improvement={
              result.current_pass_percentage - result.previous_pass_percentage
            }
          />
        </TableCell>
      </Tooltip>
    )
  }
}

Cell.propTypes = {
  result: PropTypes.object,
  colorScale: PropTypes.array,
  showFull: PropTypes.bool,
  release: PropTypes.string,
  variant: PropTypes.string,
  testName: PropTypes.string,
}

function Row(props) {
  const { columnNames, testName, results } = props

  const nameColumn = (
    <TableCell className={'cell-name'} key={testName}>
      <Tooltip title={testName}>
        <Typography className="cell-name">
          <Link
            to={pathForExactTestAnalysis(
              props.release,
              testName,
              props.excludedVariants
            )}
          >
            {testName}
          </Link>
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {props.briefTable ? '' : nameColumn}
        {columnNames.map((column, idx) => (
          <Cell
            key={'testName-' + idx}
            colorScale={props.colorScale}
            showFull={props.showFull}
            result={results[column]}
            release={props.release}
            variant={column}
            testName={testName}
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

Row.propTypes = {
  briefTable: PropTypes.bool,
  excludedVariants: PropTypes.array,
  results: PropTypes.object,
  columnNames: PropTypes.array.isRequired,
  testName: PropTypes.string.isRequired,
  colorScale: PropTypes.array.isRequired,
  showFull: PropTypes.bool,
  release: PropTypes.string.isRequired,
}

export default function TestByVariantTable(props) {
  const cookies = new Cookies()
  const cookie = cookies.get('testDetailShowFull') === 'true'
  const [showFull, setShowFull] = React.useState(
    props.showFull ? props.showFull : cookie
  )

  if (props.data === undefined || props.data.tests.length === 0) {
    return <p>No data.</p>
  }

  const handleSwitchFull = (e) => {
    cookies.set('testDetailShowFull', e.target.checked, { sameSite: true })
    setShowFull(e.target.checked)
  }

  const pageTitle = (
    <Typography variant="h4" style={{ margin: 20, textAlign: 'center' }}>
      {props.title}
    </Typography>
  )

  if (props.data.tests && Object.keys(props.data.tests).length === 0) {
    return (
      <Fragment>
        {pageTitle}
        <p>No Results.</p>
      </Fragment>
    )
  }

  if (props.data.column_names.length === 0) {
    return (
      <Typography variant="h6" style={{ marginTop: 50 }}>
        No per-variant data found.
      </Typography>
    )
  }

  const nameColumn = (
    <TableCell className={`col-name ${props.briefTable ? 'col-hide' : ''}`}>
      <FormGroup row>
        <FormControlLabel
          control={
            <Switch
              checked={showFull}
              onChange={handleSwitchFull}
              name="showFull"
            />
          }
          label="Show Full"
        />
      </FormGroup>
    </TableCell>
  )

  return (
    <div className="view" width="100%">
      {pageTitle}
      <TableContainer component="div" className="wrapper">
        <Table className="test-variant-table">
          <TableHead>
            <TableRow>
              {props.briefTable ? '' : nameColumn}
              {props.data.column_names.map((column, idx) => (
                <TableCell
                  className={'col-result' + (showFull ? '-full' : '')}
                  key={'column' + '-' + idx}
                >
                  {column}
                </TableCell>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.keys(props.data.tests).map((theTest) => (
              <Row
                briefTable={props.briefTable}
                colorScale={props.colorScale}
                showFull={showFull}
                key={theTest}
                testName={theTest}
                excludedVariants={props.excludedVariants}
                columnNames={props.data.column_names}
                results={props.data.tests[theTest]}
                release={props.release}
              />
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </div>
  )
}

TestByVariantTable.defaultProps = {
  briefTable: false,
  colorScale: [60, 100],
  excludedVariants: ['never-stable', 'aggregated'],
}

TestByVariantTable.propTypes = {
  briefTable: PropTypes.bool,
  excludedVariants: PropTypes.array,
  columnNames: PropTypes.array,
  current: PropTypes.number,
  data: PropTypes.object,
  previous: PropTypes.number,
  release: PropTypes.string.isRequired,
  results: PropTypes.object,
  testName: PropTypes.string,
  title: PropTypes.string,
  colorScale: PropTypes.array,
  showFull: PropTypes.bool,
}
