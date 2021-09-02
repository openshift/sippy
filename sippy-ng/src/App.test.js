/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import { act, waitFor } from '@testing-library/react'
import {
  expectLoadingPage,
  setupDefaultPolly,
  withoutMuiID,
} from './setupTests'
import { mount } from 'enzyme'
import { QueryParamProvider } from 'use-query-params'
import { Router } from 'react-router-dom'
import App from './App'
import React from 'react'

jest.useRealTimers()

describe('app', () => {
  setupDefaultPolly()

  beforeEach(() => {
    Date.now = jest
      .spyOn(Date, 'now')
      .mockImplementation(() => new Date(1628691480000))
  })

  it('should render correctly', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    const history = require('history').createMemoryHistory({
      basename: '/sippy-ng/',
    })
    history.push('/')

    let wrapper
    await act(async () => {
      wrapper = mount(
        <QueryParamProvider>
          <Router history={history}>
            <App />
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
    expect(wrapper.text()).toContain('Infrastructure')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(12)
  })
})
