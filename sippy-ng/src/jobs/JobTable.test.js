/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { QueryParamProvider } from 'use-query-params'
import { mount } from 'enzyme'
import { expectLoadingPage, setupDefaultPolly, withoutMuiID } from '../setupTests'
import { BrowserRouter } from 'react-router-dom'
import JobTable from './JobTable'

jest.useRealTimers()

describe('JobTable', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      wrapper = mount(
                <QueryParamProvider>
                    <BrowserRouter>
                        <JobTable release="4.8" />
                    </BrowserRouter>)
                </QueryParamProvider>)
    })

    expectLoadingPage(wrapper).toBeTruthy()

    await waitFor(() => {
      wrapper.update()
      expectLoadingPage(wrapper).toBeFalsy()
    })

    expect(wrapper.text()).toContain('-e2e-')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
