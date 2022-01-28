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
import { TestAnalysis } from './TestAnalysis'
import React from 'react'

jest.useRealTimers()

describe('TestAnalysis', () => {
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
            <TestAnalysis
              release="4.8"
              test="[sig-api-machinery] Kubernetes APIs remain available with reused connections"
            />
          </BrowserRouter>
        </QueryParamProvider>
      )
    })

    expectLoadingPage(wrapper).toBeTruthy()

    await waitFor(() => {
      wrapper.update()
      expectLoadingPage(wrapper).toBeFalsy()
    })

    expect(wrapper.text()).toContain('Pass Rate By Job')
    expect(wrapper.text()).toContain('Pass Rate By Variant')
    expect(wrapper.exists()).toBe(true)
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(3)
  })
})
