import {
  Apps,
  AppsOutage,
  BugReport,
  ChevronRight,
  Code,
  Dashboard,
  Favorite,
  FileCopyOutlined,
  GitHub,
  NotificationsActive,
  SmartToy,
} from '@mui/icons-material'
import { DEFAULT_TEST_FILTERS } from '../constants'
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
import { SippyCapabilitiesContext } from '../App'
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
import React, { Fragment, useEffect, useMemo, useRef } from 'react'
import SearchIcon from '@mui/icons-material/Search'
import SippyLogo from './SippyLogo'

const StyledListItemButton = styled(ListItemButton)({
  padding: 0,
  margin: 0,
})

function groupReleasesByProduct(releases, releaseAttrs) {
  const groups = {}
  releases.forEach((release) => {
    const product = releaseAttrs?.[release]?.product || 'Other'
    if (!groups[product]) {
      groups[product] = []
    }
    groups[product].push(release)
  })

  const sortedProducts = Object.keys(groups).sort((a, b) => {
    if (a === 'OCP') return -1
    if (b === 'OCP') return 1
    return a.localeCompare(b)
  })

  return sortedProducts.map((product) => ({
    product,
    releases: groups[product],
  }))
}

export default function Sidebar(props) {
  const classes = useTheme()
  const location = useLocation()

  const [openReleases, setOpenReleases] = React.useState({})
  const [openGroups, setOpenGroups] = React.useState({})
  const initialPathRef = useRef(location.pathname)

  const productGroups = useMemo(
    () =>
      groupReleasesByProduct(
        props.releaseConfig.releases || [],
        props.releaseConfig.release_attrs
      ),
    [props.releaseConfig]
  )

  useEffect(() => {
    const parts = initialPathRef.current.split('/')
    const tmpOpenReleases = {}
    const tmpOpenGroups = {}

    productGroups.forEach(({ product }) => {
      tmpOpenGroups[product] = product === 'OCP'
    })

    if (parts.length >= 3) {
      const release = parts[2]
      const group = productGroups.find((g) => g.releases.includes(release))
      if (group) {
        tmpOpenGroups[group.product] = true
        tmpOpenReleases[releaseKey(group.product, release)] = true
      }
    } else if (props.defaultRelease) {
      const group = productGroups.find((g) =>
        g.releases.includes(props.defaultRelease)
      )
      if (group) {
        tmpOpenGroups[group.product] = true
        tmpOpenReleases[releaseKey(group.product, props.defaultRelease)] = true
      }
    } else if (
      productGroups.length > 0 &&
      productGroups[0].releases.length > 0
    ) {
      tmpOpenGroups[productGroups[0].product] = true
      tmpOpenReleases[
        releaseKey(productGroups[0].product, productGroups[0].releases[0])
      ] = true
    }

    setOpenGroups(tmpOpenGroups)
    setOpenReleases(tmpOpenReleases)
  }, [productGroups, props.defaultRelease])

  function handleGroupClick(product) {
    setOpenGroups((prev) => ({ ...prev, [product]: !prev[product] }))
  }

  function releaseKey(product, release) {
    return `${product}::${release}`
  }

  function handleReleaseClick(product, release) {
    const key = releaseKey(product, release)
    setOpenReleases((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  function reportAnIssueURI() {
    const description = `Describe your feature request or bug:\n\n

    Relevant Sippy URL:\n
    ${window.location.href}\n\n`
    return `https://redhat.atlassian.net/secure/CreateIssueDetails!init.jspa?pid=11604&issuetype=10009&description=${safeEncodeURIComponent(
      description
    )}`
  }

  function renderReleaseItems(release) {
    const newInstall = useNewInstallTests(release)
    return (
      <List component="div" disablePadding sx={{ pl: 3 }}>
        <ListItem
          key={'release-overview-' + release}
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
        {props.releaseConfig.release_attrs?.[release]?.capabilities
          ?.payloadTags && (
          <SippyCapabilitiesContext.Consumer>
            {(value) => {
              if (value.includes('openshift_releases')) {
                return (
                  <ListItem
                    key={'release-tags-' + release}
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
          </SippyCapabilitiesContext.Consumer>
        )}
        <ListItem
          key={'release-jobs-' + release}
          component={Link}
          to={withSort(
            pathForJobsWithFilter(release, {
              items: [...withoutUnstable()],
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

        {props.releaseConfig.release_attrs?.[release]?.capabilities
          ?.pullRequests && (
          <Fragment>
            <ListItem
              key={'release-pull-requests-' + release}
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
              key={'release-repositories-' + release}
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
          key={'release-tests-' + release}
          component={Link}
          to={withSort(
            pathForTestsWithFilter(release, {
              items: DEFAULT_TEST_FILTERS,
              linkOperator: 'and',
            }),
            'net_improvement',
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

        {props.releaseConfig.release_attrs?.[release]?.capabilities
          ?.featureGates && (
          <SippyCapabilitiesContext.Consumer>
            {(value) => {
              if (value.includes('openshift_releases')) {
                return (
                  <ListItem
                    key={'release-feature-gates-' + release}
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
          </SippyCapabilitiesContext.Consumer>
        )}

        <SippyCapabilitiesContext.Consumer>
          {(value) => {
            if (value.includes('openshift_releases')) {
              return (
                <ListItem
                  key={'release-upgrade-' + release}
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
        </SippyCapabilitiesContext.Consumer>

        <SippyCapabilitiesContext.Consumer>
          {(value) => {
            if (value.includes('openshift_releases')) {
              return (
                <ListItem
                  key={'release-install-' + release}
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
        </SippyCapabilitiesContext.Consumer>

        <SippyCapabilitiesContext.Consumer>
          {(value) => {
            if (value.includes('openshift_releases')) {
              const link = newInstall
                ? pathForTestByVariant(
                    release,
                    'install should succeed: infrastructure'
                  )
                : pathForTestByVariant(
                    release,
                    '[sig-sippy] infrastructure should work'
                  )

              return (
                <ListItem
                  key={'release-infrastructure-' + release}
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
        </SippyCapabilitiesContext.Consumer>
      </List>
    )
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
      <SippyCapabilitiesContext.Consumer>
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

                  <SippyCapabilitiesContext.Consumer>
                    {(value) => {
                      if (value.includes('chat')) {
                        return (
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
                              <ListItemText primary="Chat Assistant" />
                            </StyledListItemButton>
                          </ListItem>
                        )
                      }
                    }}
                  </SippyCapabilitiesContext.Consumer>

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
                  <SippyCapabilitiesContext.Consumer>
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
                  </SippyCapabilitiesContext.Consumer>
                </List>
              </Fragment>
            )
          }
        }}
      </SippyCapabilitiesContext.Consumer>

      <SippyCapabilitiesContext.Consumer>
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
                  {productGroups.map(({ product, releases }) => (
                    <Fragment key={'product-group-' + product}>
                      <ListItem onClick={() => handleGroupClick(product)}>
                        <StyledListItemButton>
                          <ChevronRight
                            sx={{
                              transition: 'transform 0.2s',
                              transform: openGroups[product]
                                ? 'rotate(90deg)'
                                : 'none',
                            }}
                          />
                          <ListItemText
                            primary={product}
                            primaryTypographyProps={{ fontWeight: 'bold' }}
                          />
                        </StyledListItemButton>
                      </ListItem>
                      <Collapse
                        in={openGroups[product]}
                        timeout="auto"
                        unmountOnExit
                      >
                        <List component="div" disablePadding>
                          {releases.map((release) => (
                            <Fragment key={'section-release-' + release}>
                              <ListItem
                                onClick={() =>
                                  handleReleaseClick(product, release)
                                }
                                sx={{ pl: 3 }}
                              >
                                <StyledListItemButton>
                                  <ChevronRight
                                    fontSize="small"
                                    sx={{
                                      transition: 'transform 0.2s',
                                      transform: openReleases[
                                        releaseKey(product, release)
                                      ]
                                        ? 'rotate(90deg)'
                                        : 'none',
                                    }}
                                  />
                                  <ListItemText primary={release} />
                                </StyledListItemButton>
                              </ListItem>
                              <Collapse
                                in={openReleases[releaseKey(product, release)]}
                                timeout="auto"
                                unmountOnExit
                              >
                                {renderReleaseItems(release)}
                              </Collapse>
                            </Fragment>
                          ))}
                        </List>
                      </Collapse>
                    </Fragment>
                  ))}
                </List>
              </Fragment>
            )
          }
        }}
      </SippyCapabilitiesContext.Consumer>

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
