/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { mount } from 'enzyme'
import ReleaseOverview from './ReleaseOverview'
import { BrowserRouter } from 'react-router-dom'
import { QueryParamProvider } from 'use-query-params'
import { expectLoadingPage, setupDefaultPolly, withoutMuiID } from '../setupTests'

jest.useRealTimers()

describe('release-overview', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      wrapper = mount(
          <QueryParamProvider>
            <BrowserRouter>
              <ReleaseOverview release="4.8" />
            </BrowserRouter>
          </QueryParamProvider>
      )
    })

    expectLoadingPage(wrapper).toBeTruthy()
    await waitFor(() => {
      wrapper.update()
      expectLoadingPage(wrapper).toBeFalsy()
    })

    expect(wrapper.exists()).toBe(true)
    expect(wrapper.text()).toContain('e2e-gcp')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()

    // Latch the number of API calls this page makes. Increase if needed,
    // but this is used to prevent useEffect() being stuck in loops
    // due to state changes
    expect(fetchSpy).toHaveBeenCalledTimes(8)
  })
})
