import { act } from '@testing-library/react'
import { mount } from 'enzyme'
import { withoutMuiID } from '../setupTests'
import React from 'react'
import SummaryCard from './SummaryCard'

jest.useRealTimers()

describe(SummaryCard, () => {
  it('shows overview', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(
        <SummaryCard
          flakes={10}
          success={10}
          fail={10}
          threshold={{ success: 90, warning: 80, error: 0 }}
        />
      )
    })

    expect(wrapper.find('Doughnut').exists()).toBe(true)
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })
})
