/** @jest-environment setup-polly-jest/jest-environment-node */

import 'jsdom-global/register'
import { act } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'
import { mount } from 'enzyme'
import { setupPolly } from 'setup-polly-jest'
import { withoutMuiID } from '../setupTests'
import Jobs from './Jobs'
import path from 'path'
import React from 'react'

jest.useRealTimers()

describe('Jobs', () => {
  const context = setupPolly({})

  beforeEach(() => {
    context.polly.configure({
      mode: process.env.POLLY_MODE,
      adapters: [require('@pollyjs/adapter-node-http')],
      persister: require('@pollyjs/persister-fs'),
      persisterOptions: {
        fs: {
          recordingsDir: path.resolve(__dirname, '../__recordings__'),
        },
      },
    })
  })

  it('should match snapshot', async () => {
    let wrapper
    await act(async () => {
      wrapper = mount(
        <BrowserRouter>
          <Jobs release="4.8" />
        </BrowserRouter>
      )
    })

    expect(wrapper.text()).toContain('Jobs')
    expect(withoutMuiID(wrapper)).toMatchSnapshot()
  })
})
