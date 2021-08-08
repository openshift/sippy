/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import React from 'react'
import { act, waitFor } from '@testing-library/react'
import { QueryParamProvider } from 'use-query-params'
import { mount } from 'enzyme'
import { setupDefaultPolly, withoutMuiID } from '../setupTests'
import { BrowserRouter } from 'react-router-dom'
import TestTable from './TestTable'

jest.useRealTimers()

describe('TestTable', () => {
  setupDefaultPolly()

  it('should render correctly', async () => {
    const fetchSpy = jest.spyOn(global, 'fetch')

    let wrapper
    await act(async () => {
      wrapper = mount(
                <QueryParamProvider>
                    <BrowserRouter>
                        <TestTable release="4.8"/>
                    </BrowserRouter>)
                </QueryParamProvider>)
    })

    expect(wrapper.find('p').contains('Loading...')).toBeTruthy()

    await waitFor(() => {
      wrapper.update()
      expect(wrapper.find('p').contains('Loading...')).toBeFalsy()
    })

    expect(wrapper.exists()).toBe(true)
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })
})
