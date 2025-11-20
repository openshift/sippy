import { Card, Container, Grid, Tab, Typography } from '@mui/material'
import { filterFor } from '../helpers'
import { JobStackedChart } from './JobStackedChart'
import { Link } from 'react-router-dom'
import { TabContext, TabList, TabPanel } from '@mui/lab'
import JobRunsTable from './JobRunsTable'
import JobTable from './JobTable'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

export default function VariantStatus(props) {
  const [tab, setTab] = React.useState(0)

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={<Link to={'/jobs/' + props.release}>Jobs</Link>}
        currentPage="Variant Analysis"
      />
      <Container size="xl">
        <Typography variant="h4" gutterBottom style={{ textAlign: 'center' }}>
          Variant: {props.variant}
        </Typography>

        <Grid container spacing={3}>
          <Grid item md={12}>
            <Card elevation={5} style={{ padding: 20 }}>
              <Typography variant="h5">Overall</Typography>
              <JobStackedChart
                release={props.release}
                filter={{
                  items: [
                    {
                      columnField: 'variants',
                      operatorValue: 'has entry',
                      value: props.variant,
                    },
                  ],
                }}
              />
            </Card>
          </Grid>

          <Grid item md={12}>
            <Card elevation={5} style={{ padding: 20 }}>
              <TabContext value={tab}>
                <TabList
                  onChange={(e, newValue) => setTab(newValue)}
                  indicatorColor="primary"
                  variant="fullWidth"
                >
                  <Tab label="Jobs" value={0} />
                  <Tab label="Job runs" value={1} />
                </TabList>

                <TabPanel value={0}>
                  <JobTable
                    hideControls={true}
                    pageSize={5}
                    release={props.release}
                    filterModel={{
                      items: [
                        filterFor('variants', 'has entry', props.variant),
                      ],
                    }}
                  />
                </TabPanel>
                <TabPanel value={1}>
                  <JobRunsTable
                    pageSize={5}
                    hideControls={true}
                    release={props.release}
                    filterModel={{
                      items: [
                        filterFor('variants', 'has entry', props.variant),
                      ],
                    }}
                  />
                </TabPanel>
              </TabContext>
            </Card>
          </Grid>
        </Grid>
      </Container>
    </Fragment>
  )
}

VariantStatus.propTypes = {
  release: PropTypes.string,
  variant: PropTypes.string,
}
