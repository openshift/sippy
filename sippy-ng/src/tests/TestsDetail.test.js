/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { QueryParamProvider } from 'use-query-params'
import { mount } from 'enzyme'
import { setupDefaultPolly, withoutMuiID } from '../setupTests'
import { BrowserRouter } from 'react-router-dom'
import TestsDetail from './TestsDetail'

jest.useRealTimers()

describe('TestsDetail', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      wrapper = mount(
                <QueryParamProvider>
                    <BrowserRouter>
                        <TestsDetail
                            release="4.8"
                            test={['Kubernetes APIs remain available']}
                        />
                    </BrowserRouter>)
                </QueryParamProvider>)
    })

    expect(wrapper.find('div').contains('Fetching data...')).toBeTruthy()

    await waitFor(() => {
      wrapper.update()
      expect(wrapper.find('div').contains('Fetching data...')).toBeFalsy()
    }, { timeout: 3000 })

    expect(wrapper.exists()).toBe(true)
    expect(wrapper.text()).toContain('[sig-api-machinery] Kubernetes APIs remain available for new connections')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
