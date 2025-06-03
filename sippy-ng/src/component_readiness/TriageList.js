import { CapabilitiesContext } from '../App'
import { getTriagesAPIUrl } from './CompReadyUtils'
import React, { Fragment } from 'react'
import TriagedTestsPanel from './TriagedTestsPanel'

export default function TriageList() {
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [triages, setTriages] = React.useState([])
  const [message, setMessage] = React.useState('')

  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const localDBEnabled = capabilitiesContext.includes('local_db')

  React.useEffect(() => {
    setIsLoaded(false)
    let triageFetch
    // triage entries will only be available when there is a postgres connection
    if (localDBEnabled) {
      triageFetch = fetch(getTriagesAPIUrl()).then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
    } else {
      triageFetch = Promise.resolve({})
    }

    triageFetch
      .then((t) => {
        setTriages(t)
        setIsLoaded(true)
        document.title = 'All Triages'
      })
      .catch((error) => {
        setMessage(error.toString())
      })
  }, [])

  if (message !== '') {
    return <h2>{message}</h2>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <Fragment>
      <h2>All Triage Records</h2>
      <TriagedTestsPanel triageEntries={triages} triageEntriesPerPage={50} />
    </Fragment>
  )
}

TriageList.propTypes = {}
