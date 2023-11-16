import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Tooltip,
  Typography,
} from '@mui/material'
import { ExpandMore } from '@mui/icons-material'
import { pathForVariantAnalysis } from '../helpers'
import { PropTypes } from 'prop-types'
import { VARIANT_THRESHOLDS } from '../constants'
import Alert from '@mui/material/Alert'
import Grid from '@mui/material/Unstable_Grid2'
import Info from '@mui/icons-material/Info'
import MiniCard from '../components/MiniCard'
import React, { Fragment, useEffect } from 'react'

export default function VariantCards(props) {
  const [variants, setVariants] = React.useState([])
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    fetch(
      process.env.REACT_APP_API_URL + '/api/variants?release=' + props.release
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setVariants(json)
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

  const minicard = (variant, index) => {
    return (
      <Grid key={index} md={2} sm={4}>
        <MiniCard
          link={pathForVariantAnalysis(props.release, variant.name)}
          threshold={VARIANT_THRESHOLDS}
          name={variant.name}
          current={variant.current_pass_percentage}
          currentRuns={variant.current_runs}
          previous={variant.previous_pass_percentage}
          previousRuns={variant.previous_runs}
          tooltip={
            'Current runs: ' +
            variant.current_runs +
            ', previous runs: ' +
            variant.previous_runs
          }
        />
      </Grid>
    )
  }

  const cards = []
  const noData = [] // Put these at the end
  variants.forEach((variant, index) => {
    if (variant.current_runs === 0) {
      noData.push(minicard(variant, index))
    } else {
      cards.push(minicard(variant, index))
    }
  })

  return (
    <Fragment>
      <Grid md={12} sm={12}>
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
  release: PropTypes.string.isRequired,
}
