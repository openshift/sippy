import {
  convertApiUrlToUiUrl,
  getRegressionAPIUrl,
  getTestDetailsLink,
} from './CompReadyUtils'
import { useNavigate, useParams } from 'react-router'
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
        const detailKeys = regression?.links
          ? Object.keys(regression.links)
              .filter((k) => k.startsWith('test_details:'))
              .sort()
          : []
        const mainViewKey = detailKeys.find((k) => k.endsWith('-main'))
        const selectedKey = mainViewKey || detailKeys[0]
        const testDetailsUrl = selectedKey
          ? regression.links[selectedKey]
          : getTestDetailsLink(regression?.links)
        if (!testDetailsUrl) {
          setError('No test details link available for this regression.')
          return
        }
        const uiUrl = convertApiUrlToUiUrl(testDetailsUrl)
        let parsed
        try {
          parsed = new URL(uiUrl, window.location.origin)
        } catch {
          setError('Could not parse test details link.')
          return
        }
        if (!parsed.pathname.startsWith('/sippy-ng/component_readiness/')) {
          setError('Unexpected redirect path.')
          return
        }
        const navigatePath =
          parsed.pathname.replace('/sippy-ng', '') + parsed.search
        navigate(navigatePath, { replace: true })
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
