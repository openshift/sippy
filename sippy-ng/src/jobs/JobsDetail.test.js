/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { QueryParamProvider } from 'use-query-params'
import { mount } from 'enzyme'
import { setupDefaultPolly, withoutMuiID } from '../setupTests'
import { BrowserRouter } from 'react-router-dom'
import JobsDetail from './JobsDetail'

jest.useRealTimers()

describe('JobsDetail', () => {
  setupDefaultPolly()

  it('should match snapshot', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      wrapper = mount(
                <QueryParamProvider>
                    <BrowserRouter>
                        <JobsDetail release="4.8"
                                    filter="4.8-e2e-gcp-upgrade"
                        />
                    </BrowserRouter>)
                </QueryParamProvider>)
    })

    expect(wrapper.find('div').contains('Fetching data...')).toBeTruthy()

    await waitFor(() => {
      wrapper.update()
      expect(wrapper.find('div').contains('Fetching data...')).toBeFalsy()
    })

    expect(wrapper.text()).toContain('4.8-e2e-gcp-upgrade')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
