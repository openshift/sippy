import React from 'react'
import { act } from '@testing-library/react'
import { withoutMuiID } from '../setupTests'
import { mount } from 'enzyme'
import SimpleBreadcrumbs from './SimpleBreadcrumbs'
import { BrowserRouter } from 'react-router-dom'

jest.useRealTimers()

describe(SimpleBreadcrumbs, () => {
  it('shows overview', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(
                <BrowserRouter>
                    <SimpleBreadcrumbs release="4.8" currentPage="Jobs"/>
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
                    <SimpleBreadcrumbs release="4.8" currentPage="Jobs"/>
                </BrowserRouter>
      )
    })
    expect(wrapper.find('p').text()).toContain('Jobs')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })
})
