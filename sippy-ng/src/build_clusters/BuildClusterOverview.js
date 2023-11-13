import {
  Card,
  Container,
  Paper,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from '@mui/material'
import { createTheme } from '@mui/material/styles'
import { Fragment, React } from 'react'
import { Link, Route, Switch, useRouteMatch } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { TabContext } from '@mui/lab'
import BuildClusterHealthChart from './BuildClusterHealthChart'
import BuildClusterTable from './BuildClusterTable'
import Grid from '@mui/material/Grid'
import InfoIcon from '@mui/icons-material/Info'

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
  }),
  { defaultTheme }
)

export default function BuildClusterOverview(props) {
  const classes = useStyles()
  const { path, url } = useRouteMatch()

  return (
    <Container size="xl">
      <Typography align="center" variant="h4">
        Build Cluster Health Overview
      </Typography>
      <Grid container spacing={3} alignItems="stretch">
        <Grid item md={6} sm={12}>
          <Card elevation={5} style={{ padding: 20, height: '100%' }}>
            <Typography variant="h6">
              Build Cluster Pass Rate (7 Day)
              <Tooltip title={'Shows the pass rates this week vs. last week.'}>
                <InfoIcon />
              </Tooltip>
            </Typography>
            <BuildClusterTable briefTable={true} hideControls={true} />
          </Card>
        </Grid>

        <Grid item md={6} sm={12}>
          <Card elevation={5} style={{ padding: 20, height: '100%' }}>
            <Typography variant="h6">
              Build Cluster Pass Rate (2 Day)
              <Tooltip
                title={
                  'Shows the pass rates the last 2 days compared to the previous 7 days.'
                }
              >
                <InfoIcon />
              </Tooltip>
            </Typography>
            <BuildClusterTable
              briefTable={true}
              hideControls={true}
              period="twoDay"
            />
          </Card>
        </Grid>

        <Grid item md={12} sm={12}>
          <Card elevation={5} style={{ padding: 20, height: '100%' }}>
            <Typography variant="h6">
              Pass Rate By Day
              <Tooltip title={'Shows the pass rates this week vs. last week.'}>
                <InfoIcon />
              </Tooltip>
            </Typography>
            <BuildClusterHealthChart period="day" />
          </Card>
        </Grid>
      </Grid>
    </Container>
  )
}
