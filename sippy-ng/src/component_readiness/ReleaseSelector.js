import { FormControl, InputLabel, MenuItem, Select } from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import { useEffect } from 'react'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles((theme) => ({
  formControl: {
    margin: theme.spacing(1),
    minWidth: 80,
  },
  selectEmpty: {
    marginTop: theme.spacing(5),
  },
  label: {
    display: 'flex',
    whiteSpace: 'nowrap',
  },
}))

function ReleaseSelector(props) {
  const classes = useStyles()
  const [versions, setVersions] = React.useState([])
  const { label, version, onChange } = props

  const fetchData = () => {
    const useLocal = false
    if (useLocal) {
      console.log('quick mode...')
      const data = { releases: ['4.10', '4.11', '4.12', '4.13', '4.14'] }
      setVersions(data.releases)
    } else {
      fetch(process.env.REACT_APP_API_URL + '/api/releases')
        .then((response) => response.json())
        .then((data) => {
          setVersions(
            data.releases.filter((aVersion) => {
              // We won't process Presubmits or 3.11
              return aVersion !== 'Presubmits' && aVersion != '3.11'
            })
          )
        })
        .catch((error) => console.error(error))
    }
  }

  useEffect(() => {
    fetchData()
  }, [])

  const handleChange = (event) => {
    onChange(event.target.value)
  }

  // Ensure that versions has a list of versions before trying to display the Form
  if (versions.length === 0) {
    return <p>Loading Releases...</p>
  }

  return (
    <FormControl className={classes.formControl}>
      <InputLabel className={classes.label}>{label}</InputLabel>
      <Select value={version} onChange={handleChange}>
        {versions.map((v) => (
          <MenuItem key={v} value={v}>
            {v}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  )
}

ReleaseSelector.propTypes = {
  label: PropTypes.string,
  version: PropTypes.string,
  versions: PropTypes.array,
  onChange: PropTypes.func,
}

ReleaseSelector.defaultProps = {
  label: 'Version',
}

export default ReleaseSelector
