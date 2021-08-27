/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import { act, waitFor } from '@testing-library/react'
import {
  expectLoadingPage,
  setupDefaultPolly,
  withoutMuiID,
} from '../setupTests'
import { mount } from 'enzyme'
import { QueryParamProvider } from 'use-query-params'
import { Router } from 'react-router-dom'
import React from 'react'
import Upgrades from './Upgrades'

jest.useRealTimers()

describe('upgrades', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      const history = require('history').createMemoryHistory({
        basename: '/sippy-ng/',
      })
      history.push('upgrades/4.8/operators')

      wrapper = mount(
        <QueryParamProvider>
          <Router history={history}>
            <Upgrades release="4.8" />
          </Router>
        </QueryParamProvider>
      )
    })

    expectLoadingPage(wrapper).toBeTruthy()
    await waitFor(() => {
      wrapper.update()
      expectLoadingPage(wrapper).toBeFalsy()
    })

    expect(wrapper.exists()).toBe(true)
    expect(wrapper.text()).toContain(
      '[sig-api-machinery] Kubernetes APIs remain available for new connections'
    )
    expect(withoutMuiID(wrapper)).toMatchSnapshot()

    // This page should only make 1 API call.
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
