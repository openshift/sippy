/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { QueryParamProvider } from 'use-query-params'
import { mount } from 'enzyme'
import { setupDefaultPolly, withoutMuiID } from '../setupTests'
import { BrowserRouter } from 'react-router-dom'
import JobRunsTable from './JobRunsTable'

jest.useRealTimers()

describe('JobRunsTable', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    Date.now = jest.spyOn(Date, 'now').mockImplementation(() => new Date(1628691480000))
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      wrapper = mount(
                <QueryParamProvider>
                    <BrowserRouter>
                        <JobRunsTable release="4.8" />
                    </BrowserRouter>)
                </QueryParamProvider>)
    })

    expect(wrapper.text()).toContain('Fetching data')

    await waitFor(() => {
      wrapper.update()
      expect(wrapper.text()).not.toContain('Fetching data')
    })

    expect(wrapper.text()).toContain('-e2e-')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
