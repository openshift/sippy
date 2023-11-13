import { Card, Container, Tooltip, Typography } from '@mui/material'
import { createTheme, makeStyles } from '@mui/material/styles'
import { NumberParam, useQueryParam } from 'use-query-params'
import Grid from '@mui/material/Grid'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import WorkloadMetricsTable from './WorkloadMetricsTable'

export const SCALEJOBS_TOOLTIP =
  'Shows the average and max CPU and memory for core workloads during OpenShift PerfScale team job runs. Compares the last week vs the previous 30 days. Some workload metrics are a sum of all pods, possibly running on every node, which can lead to larger than expected values.'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    root: {
      flexGrow: 1,
    },
    card: {
      minWidth: 275,
      alignContent: 'center',
      margin: 'auto',
    },
    title: {
      textAlign: 'center',
    },
    warning: {
      margin: 10,
      width: '100%',
    },
  }),
  { defaultTheme }
)

export default function WorkloadMetricsOverview(props) {
  const classes = useStyles()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [dayOffset = 1, setDayOffset] = useQueryParam('dayOffset', NumberParam)

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} />
      <div className="{classes.root}" style={{ padding: 20 }}>
        <Container maxWidth="xl">
          <Typography variant="h4" gutterBottom className={classes.title}>
            {props.release} OpenShift PerfScale Job Metrics
          </Typography>
          <Grid container spacing={3} alignItems="stretch">
            <Grid item md={12} sm={12}>
              <Card elevation={5} style={{ textAlign: 'center' }}>
                <Typography style={{ textAlign: 'center' }} variant="h5">
                  Workload Metrics
                  <Tooltip title={SCALEJOBS_TOOLTIP}>
                    <InfoIcon />
                  </Tooltip>
                </Typography>

                <WorkloadMetricsTable
                  hideControls={false}
                  limit={10}
                  pageSize={5}
                  release={props.release}
                  metric="workloadCPUMillicores"
                />
              </Card>
            </Grid>
          </Grid>
        </Container>
      </div>
    </Fragment>
  )
}

WorkloadMetricsOverview.propTypes = {
  release: PropTypes.string.isRequired,
}
