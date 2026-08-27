import {
  UNSAFE_DataRouterContext,
  UNSAFE_NavigationContext,
  useLocation,
  useNavigate,
} from 'react-router'
import { useContext } from 'react'

export const ReactRouterAdapter = ({ children }) => {
  const { navigator } = useContext(UNSAFE_NavigationContext)
  const navigate = useNavigate()
  const router = useContext(UNSAFE_DataRouterContext)?.router
  const location = useLocation()

  const adapter = {
    replace(location) {
      navigate(location.search || '?', {
        replace: true,
        state: location.state,
      })
    },
    push(location) {
      navigate(location.search || '?', {
        replace: false,
        state: location.state,
      })
    },
    get location() {
      return router?.state?.location ?? navigator?.location ?? location
    },
  }

  return children(adapter)
}
