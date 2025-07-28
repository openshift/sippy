import FullCalendar from '@fullcalendar/react'

import { filterFor, safeEncodeURIComponent } from '../helpers'
import { Tooltip, Typography } from '@mui/material'
import { useNavigate } from 'react-router-dom'
import { useTheme } from '@mui/material/styles'
import Alert from '@mui/material/Alert'
import dayGridPlugin from '@fullcalendar/daygrid'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

export default function PayloadMiniCalendar(props) {
  const theme = useTheme()
  const navigate = useNavigate()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [accepted, setAccepted] = React.useState([])
  const [rejected, setRejected] = React.useState([])

  // Link to payloads page listing the payloads for the day clicked
  const eventClick = (info) => {
    let filter = encodeURIComponent(
      JSON.stringify({
        items: [
          filterFor('architecture', 'equals', props.arch),
          filterFor('stream', 'equals', props.stream),
          filterFor(
            'release_time',
            '>=',
            `${new Date(info.event.start.getTime()).toISOString()}`
          ),
          filterFor(
            'release_time',
            '<',
            `${new Date(
              info.event.start.getTime() + 24 * 60 * 60 * 1000
            ).toISOString()}`
          ),
        ],
      })
    )

    navigate(
      `/release/${props.release}/streams/${props.arch}/${props.stream}/payloads?filters=${filter}`
    )
  }

  const fetchData = () => {
    let filter = safeEncodeURIComponent(
      JSON.stringify({
        items: [
          filterFor('architecture', 'equals', props.arch),
          filterFor('stream', 'equals', props.stream),
        ],
      })
    )

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/releases/tags/events?release=' +
        props.release +
        '&filter=' +
        filter
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        // Since we are only highlighting the square, we want 1 event per day.
        // Prefer accepted payloads to make the square green if we had any accepted payload that day.
        // Rejected payloads will make the square red, and no payload at all will be an empty square.
        let eventsOnePerDay = {}
        json.forEach((event) => {
          if (
            event.phase === 'Accepted' ||
            eventsOnePerDay[event.start] === undefined
          ) {
            event.display = 'background'
            event.title = event.phase.charAt(0) // Show 'A' for accepted for 'R' for rejected
            eventsOnePerDay[event.start] = event
          }
        })
        setAccepted(
          Object.values(eventsOnePerDay).filter((e) => e.phase === 'Accepted')
        )
        setRejected(
          Object.values(eventsOnePerDay).filter((e) => e.phase === 'Rejected')
        )
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve release tag data ' + props.release + ', ' + error
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

  return (
    <Fragment>
      <Tooltip
        title={
          <Fragment>
            <p>
              Shows a daily summary of release payloads. All dates are
              calculated based on UTC. Click the Calendar tab for a list of all
              payloads for each day.
            </p>

            <p>
              <h3>Legend</h3>
              <ul>
                <li>
                  <b>Green A</b> for any day with at least 1 accepted payload
                </li>
                <li>
                  <b> Red R</b> for any day with no accepted payload and at
                  least 1 rejection.
                </li>
                <li>
                  <b> Blank squares</b> indicate no payload for that day.
                </li>
              </ul>
            </p>
          </Fragment>
        }
      >
        <Typography variant="h6">
          Calendar Summary <InfoIcon />
        </Typography>
      </Tooltip>

      <FullCalendar
        timeZone="UTC"
        headerToolbar={{
          start: 'title',
          center: '',
          end: 'prev,next',
        }}
        plugins={[dayGridPlugin]}
        initialView="dayGridMonth"
        eventClick={eventClick}
        eventSources={[
          {
            events: accepted,
            color: theme.palette.success.light,
            textColor: theme.palette.success.contrastText,
          },
          {
            events: rejected,
            color: theme.palette.error.light,
            textColor: theme.palette.success.contrastText,
          },
        ]}
      />
    </Fragment>
  )
}

PayloadMiniCalendar.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string,
  stream: PropTypes.string,
  view: PropTypes.string,
}
