/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import { act, waitFor } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'
import {
  expectLoadingPage,
  setupDefaultPolly,
  withoutMuiID,
} from '../setupTests'
import { mount } from 'enzyme'
import { QueryParamProvider } from 'use-query-params'
import React from 'react'
import ReleaseOverview from './ReleaseOverview'

jest.useRealTimers()

describe('release-overview', () => {
  setupDefaultPolly()

  beforeEach(() => {
    Date.now = jest
      .spyOn(Date, 'now')
      .mockImplementation(() => new Date(1628691480000))
  })

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
    expect(fetchSpy).toHaveBeenCalledTimes(9)
  })
})
