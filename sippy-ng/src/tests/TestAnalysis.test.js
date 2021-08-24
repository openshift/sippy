/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { QueryParamProvider } from 'use-query-params'
import { mount } from 'enzyme'
import { expectLoadingPage, setupDefaultPolly, withoutMuiID } from '../setupTests'
import { BrowserRouter } from 'react-router-dom'
import { TestAnalysis } from './TestAnalysis'

jest.useRealTimers()

describe('TestAnalysis', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    Date.now = jest.spyOn(Date, 'now').mockImplementation(() => new Date(1628691480000))
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      wrapper = mount(
        <QueryParamProvider>
          <BrowserRouter>
            <TestAnalysis
              release="4.8"
              test="[sig-api-machinery] Kubernetes APIs remain available with reused connections"
            />
          </BrowserRouter>
        </QueryParamProvider>)
    })

    expectLoadingPage(wrapper).toBeTruthy()

    await waitFor(() => {
      wrapper.update()
      expectLoadingPage(wrapper).toBeFalsy()
    })

    expect(wrapper.text()).toContain('Pass rate')
    expect(wrapper.exists()).toBe(true)
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(3)
  })
})
