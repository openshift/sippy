import { getRegressionAPIUrl } from './CompReadyUtils'
import { useNavigate, useParams } from 'react-router-dom'
import Alert from '@mui/material/Alert'
import React from 'react'
import Typography from '@mui/material/Typography'

export default function RegressionRedirect() {
  const { regressionId } = useParams()
  const navigate = useNavigate()
  const [error, setError] = React.useState(null)

  React.useEffect(() => {
    setError(null)
    const abortController = new AbortController()

    fetch(getRegressionAPIUrl(regressionId), {
      signal: abortController.signal,
    })
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
      .then((regression) => {
        if (!regression?.links?.test_details) {
          setError('No test details link available for this regression.')
          return
        }
        const apiIndex = regression.links.test_details.indexOf('/api/')
        if (apiIndex === -1) {
          setError('Could not parse test details link.')
          return
        }
        const pathAfterApi = regression.links.test_details.substring(
          apiIndex + 5
        )
        let parsed
        try {
          parsed = new URL(pathAfterApi, window.location.origin)
        } catch {
          setError('Could not parse test details link.')
          return
        }
        if (!parsed.pathname.startsWith('/component_readiness/')) {
          setError('Unexpected redirect path.')
          return
        }
        navigate(parsed.pathname + parsed.search, { replace: true })
      })
      .catch((err) => {
        if (err.name === 'AbortError') {
          return
        }
        setError('Failed to load regression: ' + err.message)
      })
    return () => {
      abortController.abort()
    }
  }, [regressionId, navigate])

  if (error) {
    return <Alert severity="error">{error}</Alert>
  }

  return <Typography>Loading regression details...</Typography>
}
