/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import { act, waitFor } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'
import {
  expectLoadingPage,
  setupDefaultPolly,
  withoutMuiID,
} from '../setupTests'
import { JobAnalysis } from './JobAnalysis'
import { mount } from 'enzyme'
import { QueryParamProvider } from 'use-query-params'
import React from 'react'

jest.useRealTimers()

describe('JobAnalysis', () => {
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
            <JobAnalysis release="4.8" />
          </BrowserRouter>
        </QueryParamProvider>
      )
    })

    expectLoadingPage(wrapper).toBeTruthy()

    await waitFor(() => {
      wrapper.update()
      expectLoadingPage(wrapper).toBeFalsy()
    })

    expect(wrapper.text()).toContain('Job results')
    expect(wrapper.exists()).toBe(true)
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
