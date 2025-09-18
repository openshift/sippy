import {
  Apps,
  AppsOutage,
  BugReport,
  Code,
  Dashboard,
  ExpandLess,
  ExpandMore,
  Favorite,
  FileCopyOutlined,
  GitHub,
  NotificationsActive,
  SmartToy,
} from '@mui/icons-material'
import { BOOKMARKS } from '../constants'
import { CapabilitiesContext } from '../App'
import { LaunderedListItem } from './Laundry'
import { Link, useLocation } from 'react-router-dom'
import { ListItemButton, ListSubheader, useTheme } from '@mui/material'
import {
  pathForJobsWithFilter,
  pathForTestByVariant,
  pathForTestsWithFilter,
  safeEncodeURIComponent,
  useNewInstallTests,
  withoutUnstable,
  withSort,
} from '../helpers'
import { styled } from '@mui/styles'
import ApartmentIcon from '@mui/icons-material/Apartment'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import Collapse from '@mui/material/Collapse'
import Divider from '@mui/material/Divider'
import ExitToAppIcon from '@mui/icons-material/ExitToApp'
import HomeIcon from '@mui/icons-material/Home'
import InfoIcon from '@mui/icons-material/Info'
import List from '@mui/material/List'
import ListIcon from '@mui/icons-material/List'
import ListItem from '@mui/material/ListItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SearchIcon from '@mui/icons-material/Search'
import SippyLogo from './SippyLogo'

const StyledListItemButton = styled(ListItemButton)({
  padding: 0,
  margin: 0,
})

export default function Sidebar(props) {
  const classes = useTheme()
  const location = useLocation()

  const [open, setOpen] = React.useState({})

  useEffect(() => {
    return () => {
      // infer release from current url when loading sidebar for first time
      let parts = location.pathname.split('/')
      let tmpOpen = open
      if (parts.length >= 3) {
        let index = props.releaseConfig.releases.indexOf(parts[2])
        if (index !== -1) {
          tmpOpen[index] = true
        }
      } else {
        let defaultIndex = 0
        if (props.defaultRelease != undefined) {
          defaultIndex = props.releaseConfig.releases.indexOf(
            props.defaultRelease
          )
          if (defaultIndex < 0) {
            defaultIndex = 0
          }
        }
        tmpOpen[defaultIndex] = true
      }
      setOpen(tmpOpen)
    }
  }, [props])

  function handleClick(id) {
    setOpen((prevState) => ({ ...prevState, [id]: !prevState[id] }))
  }

  function reportAnIssueURI() {
    const description = `Describe your feature request or bug:\n\n
    
    Relevant Sippy URL:\n
    ${window.location.href}\n\n`
    return `https://issues.redhat.com/secure/CreateIssueDetails!init.jspa?priority=10200&pid=12323832&issuetype=17&description=${safeEncodeURIComponent(
      description
    )}`
  }

  return (
    <Fragment>
      <List>
        <ListItem component={Link} to="/" key="Home">
          <StyledListItemButton>
            <ListItemIcon>
              <HomeIcon />
            </ListItemIcon>
            <ListItemText primary="Home" />
          </StyledListItemButton>
        </ListItem>
      </List>
      <CapabilitiesContext.Consumer>
        {(value) => {
          if (value.includes('openshift_releases')) {
            return (
              <Fragment>
                <Divider />
                <List
                  subheader={
                    <ListSubheader component="div" id="Overall Components">
                      Tools
                    </ListSubheader>
                  }
                >
                  <ListItem
                    key={'release-health-'}
                    component={Link}
                    to={'/component_readiness/main'}
                    className={classes.nested}
                  >
                    <StyledListItemButton>
                      <ListItemIcon>
                        <AppsOutage />
                      </ListItemIcon>
                      <ListItemText primary="Component Readiness" />
                    </StyledListItemButton>
                  </ListItem>

                  {/* Only show Chat Agent if REACT_APP_CHAT_API_URL is set */}
                  {process.env.REACT_APP_CHAT_API_URL && (
                    <ListItem
                      key={'chat-agent'}
                      component={Link}
                      to={'/chat'}
                      className={classes.nested}
                    >
                      <StyledListItemButton>
                        <ListItemIcon>
                          <SmartToy />
                        </ListItemIcon>
                        <ListItemText primary="Chat Agent" />
                      </StyledListItemButton>
                    </ListItem>
                  )}

                  <ListItem
                    component="a"
                    target="_blank"
                    href="https://alertmanager-trt-service.dptools.openshift.org/#/alerts?receiver=trt-monitoring-trt-trt-alerts-slack-notifications"
                    key="Alerts"
                  >
                    <ListItemIcon>
                      <NotificationsActive />
                    </ListItemIcon>
                    <ListItemText primary="Alert Manager" />
                  </ListItem>

                  <ListItem
                    component="a"
                    target="_blank"
                    href="https://grafana-loki.ci.openshift.org/dashboards/f/4X8Jfhs4z/openshift-ci-observability"
                    key="ObservabilityDashboard"
                  >
                    <ListItemIcon>
                      <Dashboard />
                    </ListItemIcon>
                    <ListItemText primary="CI Observability Dashboards" />
                  </ListItem>
                  <CapabilitiesContext.Consumer>
                    {(value) => {
                      if (value.includes('build_clusters')) {
                        return (
                          <Fragment>
                            <ListItem
                              key={'build-cluster-health'}
                              component={Link}
                              to={`/build_clusters`}
                              className={classes.nested}
                            >
                              <StyledListItemButton>
                                <ListItemIcon>
                                  <Favorite />
                                </ListItemIcon>
                                <ListItemText primary="Build Cluster Health" />
                              </StyledListItemButton>
                            </ListItem>
                          </Fragment>
                        )
                      }
                    }}
                  </CapabilitiesContext.Consumer>
                </List>
              </Fragment>
            )
          }
        }}
      </CapabilitiesContext.Consumer>

      <CapabilitiesContext.Consumer>
        {(value) => {
          if (value.includes('local_db')) {
            return (
              <Fragment>
                <Divider />
                <List
                  subheader={
                    <ListSubheader component="div" id="releases">
                      Releases
                    </ListSubheader>
                  }
                >
                  {props.releaseConfig.releases.map((release, index) => (
                    <Fragment key={'section-release-' + index}>
                      <ListItem
                        key={'item-release-' + index}
                        onClick={() => handleClick(index)}
                      >
                        <StyledListItemButton>
                          {open[index] ? <ExpandLess /> : <ExpandMore />}
                          <ListItemText primary={release} />
                        </StyledListItemButton>
                      </ListItem>
                      <Collapse in={open[index]} timeout="auto" unmountOnExit>
                        <List component="div" disablePadding>
                          <ListItem
                            key={'release-overview-' + index}
                            component={Link}
                            to={'/release/' + release}
                            className={classes.nested}
                          >
                            <StyledListItemButton>
                              <ListItemIcon>
                                <InfoIcon />
                              </ListItemIcon>
                              <ListItemText primary="Overview" />
                            </StyledListItemButton>
                          </ListItem>
                          {props.releaseConfig.release_attrs?.[release]
                            ?.capabilities?.payloadTags && (
                            <CapabilitiesContext.Consumer>
                              {(value) => {
                                if (value.includes('openshift_releases')) {
                                  return (
                                    <ListItem
                                      key={'release-tags-' + index}
                                      component={Link}
                                      to={`/release/${release}/streams`}
                                      className={classes.nested}
                                    >
                                      <StyledListItemButton>
                                        <ListItemIcon>
                                          <FileCopyOutlined />
                                        </ListItemIcon>
                                        <ListItemText primary="Payload Streams" />
                                      </StyledListItemButton>
                                    </ListItem>
                                  )
                                }
                              }}
                            </CapabilitiesContext.Consumer>
                          )}
                          <ListItem
                            key={'release-jobs-' + index}
                            component={Link}
                            to={withSort(
                              pathForJobsWithFilter(release, {
                                items: [BOOKMARKS.RUN_7, ...withoutUnstable()],
                              }),
                              'net_improvement',
                              'asc'
                            )}
                            className={classes.nested}
                          >
                            <StyledListItemButton>
                              <ListItemIcon>
                                <ListIcon />
                              </ListItemIcon>
                              <ListItemText primary="Jobs" />
                            </StyledListItemButton>
                          </ListItem>

                          {props.releaseConfig.release_attrs?.[release]
                            ?.capabilities?.pullRequests && (
                            <Fragment>
                              <ListItem
                                key={'release-pull-requests-' + index}
                                component={Link}
                                to={`/pull_requests/${release}`}
                                className={classes.nested}
                              >
                                <StyledListItemButton>
                                  <ListItemIcon>
                                    <GitHub />
                                  </ListItemIcon>
                                  <ListItemText primary="Pull Requests" />
                                </StyledListItemButton>
                              </ListItem>
                              <ListItem
                                key={'release-repositories-' + index}
                                component={Link}
                                to={`/repositories/${release}`}
                                className={classes.nested}
                              >
                                <StyledListItemButton>
                                  <ListItemIcon>
                                    <Code />
                                  </ListItemIcon>
                                  <ListItemText primary="Repositories" />
                                </StyledListItemButton>
                              </ListItem>
                            </Fragment>
                          )}

                          <ListItem
                            key={'release-tests-' + index}
                            component={Link}
                            to={withSort(
                              pathForTestsWithFilter(release, {
                                items: [
                                  BOOKMARKS.RUN_7,
                                  BOOKMARKS.NO_NEVER_STABLE,
                                  BOOKMARKS.NO_AGGREGATED,
                                  BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
                                  BOOKMARKS.NO_STEP_GRAPH,
                                  BOOKMARKS.NO_OPENSHIFT_TESTS_SHOULD_WORK,
                                  BOOKMARKS.NO_100_FLAKE,
                                ],
                                linkOperator: 'and',
                              }),
                              'net_improvement', // sort by tests that have recently regressed the most
                              'asc'
                            )}
                            className={classes.nested}
                          >
                            <StyledListItemButton>
                              <ListItemIcon>
                                <SearchIcon />
                              </ListItemIcon>
                              <ListItemText primary="Tests" />
                            </StyledListItemButton>
                          </ListItem>

                          {props.releaseConfig.release_attrs?.[release]
                            ?.capabilities?.featureGates && (
                            <CapabilitiesContext.Consumer>
                              {(value) => {
                                if (value.includes('openshift_releases')) {
                                  return (
                                    <ListItem
                                      key={'release-feature-gates-' + index}
                                      component={Link}
                                      to={'/feature_gates/' + release}
                                      className={classes.nested}
                                    >
                                      <StyledListItemButton>
                                        <ListItemIcon>
                                          <Apps />
                                        </ListItemIcon>
                                        <ListItemText primary="Feature Gates" />
                                      </StyledListItemButton>
                                    </ListItem>
                                  )
                                }
                              }}
                            </CapabilitiesContext.Consumer>
                          )}

                          <CapabilitiesContext.Consumer>
                            {(value) => {
                              if (value.includes('openshift_releases')) {
                                return (
                                  <ListItem
                                    key={'release-upgrade-' + index}
                                    component={Link}
                                    to={'/upgrade/' + release}
                                    className={classes.nested}
                                  >
                                    <StyledListItemButton>
                                      <ListItemIcon>
                                        <ArrowUpwardIcon />
                                      </ListItemIcon>
                                      <ListItemText primary="Upgrade" />
                                    </StyledListItemButton>
                                  </ListItem>
                                )
                              }
                            }}
                          </CapabilitiesContext.Consumer>

                          <CapabilitiesContext.Consumer>
                            {(value) => {
                              if (value.includes('openshift_releases')) {
                                return (
                                  <ListItem
                                    key={'release-install-' + index}
                                    component={Link}
                                    to={'/install/' + release}
                                    className={classes.nested}
                                  >
                                    <StyledListItemButton>
                                      <ListItemIcon>
                                        <ExitToAppIcon />
                                      </ListItemIcon>
                                      <ListItemText primary="Install" />
                                    </StyledListItemButton>
                                  </ListItem>
                                )
                              }
                            }}
                          </CapabilitiesContext.Consumer>

                          <CapabilitiesContext.Consumer>
                            {(value) => {
                              if (value.includes('openshift_releases')) {
                                let newInstall = useNewInstallTests(release)
                                let link
                                if (newInstall) {
                                  link = pathForTestByVariant(
                                    release,
                                    'install should succeed: infrastructure'
                                  )
                                } else {
                                  link = pathForTestByVariant(
                                    release,
                                    '[sig-sippy] infrastructure should work'
                                  )
                                }

                                return (
                                  <ListItem
                                    key={'release-infrastructure-' + index}
                                    component={Link}
                                    to={link}
                                    className={classes.nested}
                                  >
                                    <StyledListItemButton>
                                      <ListItemIcon>
                                        <ApartmentIcon />
                                      </ListItemIcon>
                                      <ListItemText primary="Infrastructure" />
                                    </StyledListItemButton>
                                  </ListItem>
                                )
                              }
                            }}
                          </CapabilitiesContext.Consumer>
                        </List>
                      </Collapse>
                    </Fragment>
                  ))}
                </List>
              </Fragment>
            )
          }
        }}
      </CapabilitiesContext.Consumer>

      <Divider />
      <List
        subheader={
          <ListSubheader component="div" id="resources">
            Resources
          </ListSubheader>
        }
      >
        <LaunderedListItem
          component="a"
          target="_blank"
          address={reportAnIssueURI()}
          key="ReportAnIssue"
        >
          <ListItemIcon>
            <BugReport />
          </ListItemIcon>
          <ListItemText primary="Report an Issue" />
        </LaunderedListItem>

        <ListItem
          component="a"
          target="_blank"
          href="https://www.github.com/openshift/sippy"
          key="GitHub"
        >
          <ListItemIcon>
            <GitHub />
          </ListItemIcon>
          <ListItemText primary="GitHub Repo" />
        </ListItem>
        <Divider />
        <div align="center">
          <SippyLogo />
        </div>
      </List>
    </Fragment>
  )
}

Sidebar.propTypes = {
  releaseConfig: PropTypes.object,
  defaultRelease: PropTypes.string,
}
