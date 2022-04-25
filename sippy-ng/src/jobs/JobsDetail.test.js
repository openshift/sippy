/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import { act, waitFor } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'
import { mount } from 'enzyme'
import { QueryParamProvider } from 'use-query-params'
import { setupDefaultPolly, withoutMuiID } from '../setupTests'
import JobsDetail from './JobsDetail'
import React from 'react'

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
            <JobsDetail release="4.8" filter="4.8-e2e-gcp-upgrade" />
          </BrowserRouter>
          )
        </QueryParamProvider>
      )
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
