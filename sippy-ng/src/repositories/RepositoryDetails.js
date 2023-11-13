import { Card, Container, Grid, Typography } from '@mui/material'
import { createTheme, makeStyles } from '@mui/material/styles'
import { filterFor, pathForJobsWithFilter, withSort } from '../helpers'
import { Link } from 'react-router-dom'
import JobTable from '../jobs/JobTable'
import PropTypes from 'prop-types'
import PullRequestsTable from '../pull_requests/PullRequestsTable'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

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

export default function RepositoryDetails(props) {
  const classes = useStyles()

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Repositories > ${props.org}/${props.repo}`
  }, [])

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        previousPage={
          <Link to={`/repositories/${props.release}`}>Repositories</Link>
        }
        currentPage={`${props.org}/${props.repo}`}
      />
      <div className="{classes.root}" style={{ padding: 20 }}>
        <Container maxWidth="lg">
          <Typography variant="h4" gutterBottom className={classes.title}>
            {props.org}/{props.repo}
          </Typography>

          <Grid container spacing={3} alignItems="stretch">
            <Grid item md={12} sm={12}>
              <Card elevation={5} style={{ padding: 20, height: '100%' }}>
                <Typography variant="h6">
                  <Link
                    to={withSort(
                      pathForJobsWithFilter(props.release, {
                        items: [
                          filterFor('org', 'equals', props.org),
                          filterFor('repo', 'equals', props.repo),
                        ],
                      }),
                      'average_retests_to_merge',
                      'desc'
                    )}
                  >
                    Presubmit jobs
                  </Link>
                </Typography>
                <JobTable
                  view="Pull Requests"
                  sortField="average_retests_to_merge"
                  sort="desc"
                  pageSize={5}
                  hideControls={true}
                  release={props.release}
                  filterModel={{
                    items: [
                      filterFor('org', 'equals', props.org),
                      filterFor('repo', 'equals', props.repo),
                    ],
                  }}
                />
              </Card>
            </Grid>

            <Grid item md={12} sm={12}>
              <Card elevation={5} style={{ padding: 20, height: '100%' }}>
                <Typography variant="h6">
                  <Link
                    to={withSort(
                      pathForJobsWithFilter(props.release, {
                        items: [
                          filterFor('org', 'equals', props.org),
                          filterFor('repo', 'equals', props.repo),
                        ],
                      }),
                      'average_retests_to_merge',
                      'desc'
                    )}
                  >
                    Recently merged PR
                  </Link>
                </Typography>
                <PullRequestsTable
                  view="Summary"
                  pageSize={5}
                  hideControls={true}
                  release={props.release}
                  filterModel={{
                    items: [
                      filterFor('org', 'equals', props.org),
                      filterFor('repo', 'equals', props.repo),
                      filterFor('merged_at', 'is not empty'),
                    ],
                  }}
                />
              </Card>
            </Grid>
          </Grid>
        </Container>
      </div>
    </Fragment>
  )
}

RepositoryDetails.propTypes = {
  org: PropTypes.string.isRequired,
  repo: PropTypes.string.isRequired,
  release: PropTypes.string.isRequired,
}
