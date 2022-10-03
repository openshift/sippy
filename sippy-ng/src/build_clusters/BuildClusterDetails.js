import { Card, Container, Grid, Typography } from '@material-ui/core'
import { filterFor } from '../helpers'
import { Fragment, React } from 'react'
import { JobStackedChart } from '../jobs/JobStackedChart'
import JobRunsTable from '../jobs/JobRunsTable'
import PropTypes from 'prop-types'

export default function BuildClusterDetails(props) {
  return (
    <Container size="xl">
      <Grid container spacing={3} alignItems="stretch">
        <Grid item md={12} sm={12}>
          <Card elevation={5} style={{ padding: 20, height: '100%' }}>
            <Typography variant="h6">Job Runs On {props.cluster}</Typography>
            <JobRunsTable
              pageSize={10}
              hideControls={true}
              filterModel={{
                items: [
                  filterFor('cluster', 'equals', props.cluster),
                  filterFor(
                    'timestamp',
                    '>',
                    `${new Date(
                      Date.now() - 14 * 24 * 60 * 60 * 1000
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
