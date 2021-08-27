import { act } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'
import { mount } from 'enzyme'
import { withoutMuiID } from '../setupTests'
import React from 'react'
import SimpleBreadcrumbs from './SimpleBreadcrumbs'

jest.useRealTimers()

describe(SimpleBreadcrumbs, () => {
  it('shows overview', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(
        <BrowserRouter>
          <SimpleBreadcrumbs release="4.8" currentPage="Jobs" />
        </BrowserRouter>
      )
    })
    expect(wrapper.find('a[href="/release/4.8"]').exists()).toBe(true)
    expect(wrapper.find('p').text()).toContain('Jobs')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })

  it('shows current page', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(
        <BrowserRouter>
          <SimpleBreadcrumbs release="4.8" currentPage="Jobs" />
        </BrowserRouter>
      )
    })
    expect(wrapper.find('p').text()).toContain('Jobs')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })
})
