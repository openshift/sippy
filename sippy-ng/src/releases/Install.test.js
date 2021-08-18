/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { mount } from 'enzyme'
import { Router } from 'react-router-dom'
import { QueryParamProvider } from 'use-query-params'
import { expectLoadingPage, setupDefaultPolly, withoutMuiID } from '../setupTests'
import Install from './Install'

jest.useRealTimers()

describe('install', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      const history = require('history').createMemoryHistory({ basename: '/sippy-ng/' })
      history.push('install/4.8/operators')

      wrapper = mount(
                <QueryParamProvider>
                    <Router history={history}>
                        <Install release="4.8" />
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
    expect(wrapper.text()).toContain('kube-apiserver')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
