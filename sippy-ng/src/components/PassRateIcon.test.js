import { act } from '@testing-library/react'
import { mount } from 'enzyme'
import { withoutMuiID } from '../setupTests'
import PassRateIcon from './PassRateIcon'
import React from 'react'

jest.useRealTimers()

describe(PassRateIcon, () => {
  it('shows sync icon for nearly no change', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(<PassRateIcon improvement={1} />)
    })

    expect(wrapper.find('svg[data-icon="SyncAltRoundedIcon"]').exists()).toBe(
      true
    )
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })

  it('shows up arrow for improvement', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(<PassRateIcon improvement={3} />)
    })

    expect(
      wrapper.find('svg[data-icon="ArrowUpwardRoundedIcon"]').exists()
    ).toBe(true)
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })

  it('shows down arrow for regression', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(<PassRateIcon improvement={-3} />)
    })

    expect(
      wrapper.find('svg[data-icon="ArrowDownwardRoundedIcon"]').exists()
    ).toBe(true)
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })

  it('renders tooltip', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(<PassRateIcon improvement={-3.9999999} tooltip={true} />)
    })

    expect(wrapper.exists()).toBe(true)
    expect(wrapper.find('ForwardRef(Tooltip)').props().title).toBe('-4.00%')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })
})
