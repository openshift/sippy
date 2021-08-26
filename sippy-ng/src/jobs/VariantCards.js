import { Accordion, AccordionDetails, AccordionSummary, Grid, Tooltip, Typography } from '@material-ui/core'
import { ExpandMore } from '@material-ui/icons'
import Info from '@material-ui/icons/Info'
import { Alert } from '@material-ui/lab'
import { PropTypes } from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import MiniCard from '../components/MiniCard'
import { VARIANT_THRESHOLDS } from '../constants'
import { pathForVariantAnalysis } from '../helpers'

export default function VariantCards (props) {
  const [jobs, setJobs] = React.useState([])
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    fetch(process.env.REACT_APP_API_URL + '/json?release=' + props.release)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then(json => {
        setJobs(json[props.release].jobPassRateByVariant)
        setLoaded(true)
      }).catch(error => {
        setFetchError('Could not retrieve release ' + props.release + ', ' + error)
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

  const minicard = (job, index) => {
    return (
      <Grid item key={index} md={2} sm={4}>
        <MiniCard
          link={pathForVariantAnalysis(props.release, job.platform)}
          threshold={VARIANT_THRESHOLDS}
          name={job.platform}
          current={job.passRates.latest.percentage}
          currentRuns={job.passRates.latest.runs}
          previous={job.passRates.prev.percentage}
          previousRuns={job.passRates.prev.runs}
          tooltip={'Current runs: ' + job.passRates.latest.runs + ', previous runs: ' + job.passRates.prev.runs}
        />
      </Grid>
    )
  }

  const cards = []
  const noData = [] // Put these at the end
  jobs.forEach((job, index) => {
    if (job.passRates.latest.runs === 0) {
      noData.push(minicard(job, index))
    } else {
      cards.push(minicard(job, index))
    }
  })

  return (
    <Fragment>
      <Grid item md={12} sm={12}>
        <Typography variant="h5">
          Variant status
          <Tooltip title="Variant status shows the current and previous pass rates for each variant. A variant is jobs grouped by platform, SDN, architecture, etc.">
            <Info />
          </Tooltip>
        </Typography>
      </Grid>
      <Accordion elevation={5} style={{ margin: 10, borderRadius: 5 }}>
        <AccordionSummary
          aria-controls="variant-content"
          id="variant-header"
          expandIcon={<ExpandMore />}
        >
          <Grid container spacing={3}>
            {cards.slice(0, 6).map((card) => card)}
          </Grid>
        </AccordionSummary>
        <AccordionDetails>
          <Grid container spacing={3} style={{ marginRight: 20 }}>
            {cards.length > 6 ? cards.slice(6).map((card) => card) : ''}
            {noData.map((card) => card)}
          </Grid>
        </AccordionDetails>
      </Accordion>
    </Fragment>
  )
}

VariantCards.propTypes = {
  release: PropTypes.string.isRequired
}
