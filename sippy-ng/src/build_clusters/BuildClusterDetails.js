import { Card, Container, Grid, Typography } from '@material-ui/core'
import { filterFor, getReportStartDate, not } from '../helpers'
import { JobStackedChart } from '../jobs/JobStackedChart'
import { ReportEndContext } from '../App'
import JobRunsTable from '../jobs/JobRunsTable'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function BuildClusterDetails(props) {
  const startDate = getReportStartDate(React.useContext(ReportEndContext))
  return (
    <Container size="xl">
      <Grid container spacing={3} alignItems="stretch">
        <Grid item md={12} sm={12}>
          <Card elevation={5} style={{ padding: 20, height: '100%' }}>
            <Typography variant="h6">
              Periodic Job Runs On {props.cluster}
            </Typography>
            <JobRunsTable
              pageSize={10}
              hideControls={true}
              filterModel={{
                items: [
                  filterFor('cluster', 'equals', props.cluster),
                  not(filterFor('name', 'starts with', 'pull-ci')),
                  filterFor(
                    'timestamp',
                    '>',
                    `${new Date(
                      startDate - 14 * 24 * 60 * 60 * 1000
                    ).getTime()}`
                  ),
                ],
              }}
            />
          </Card>
        </Grid>
      </Grid>
    </Container>
  )
}

BuildClusterDetails.propTypes = {
  cluster: PropTypes.string,
}
