import React from 'react'
import { act } from '@testing-library/react'
import { withoutMuiID } from '../setupTests'
import LastUpdated from './LastUpdated'
import { mount } from 'enzyme'

const minute = 60 * 1000
const hour = minute * 60

jest.useRealTimers()

describe(LastUpdated, () => {
  it('shows unknown when last update is unknown', async () => {
    let wrapper
    Date.now = jest.fn(() => 0)

    await act(async () => {
      wrapper = mount(<LastUpdated lastUpdated={new Date(0)} />)
    })

    expect(wrapper.html()).toContain('Last update unknown')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })

  it('shows minutes when < an hour', async () => {
    const lastUpdated = new Date(0)
    Date.now = jest.fn(() => 10 * minute)

    let wrapper
    await act(async () => {
      wrapper = mount(<LastUpdated lastUpdated={lastUpdated} />)
    })

    expect(wrapper.html()).toContain('Last updated 10 minutes ago')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })

  it('shows hours when > an hour', async () => {
    const lastUpdated = new Date(0)
    Date.now = jest.fn(() => 3 * hour)

    let wrapper
    await act(async () => {
      wrapper = mount(<LastUpdated lastUpdated={lastUpdated} />)
    })

    expect(wrapper.exists()).toBe(true)
    expect(wrapper.html()).toContain('Last updated 3 hours ago')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })
})
