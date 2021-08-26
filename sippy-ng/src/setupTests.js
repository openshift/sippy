/** @jest-environment setup-polly-jest/jest-environment-node */

import '@testing-library/jest-dom'
import Enzyme from 'enzyme'
import Adapter from '@wojtekmaj/enzyme-adapter-react-17'
import fetch from 'node-fetch'
import path from 'path'
import { setupPolly } from 'setup-polly-jest'
import toDiffableHtml from 'diffable-html'

// https://github.com/mui-org/material-ui/issues/21293
export const withoutMuiID = (wrapper) => toDiffableHtml(wrapper.html()
  .replace(/id="mui-[0-9]*"/g, '')
  .replace(/aria-labelledby="(mui-[0-9]* *)*"/g, ''))

Enzyme.configure({ adapter: new Adapter() })

// To stub out fetch(), we need to use node-fetch and this line:
global.fetch = fetch

// See: https://github.com/vuejs/vue-test-utils/issues/974
global.requestAnimationFrame = cb => cb()

// When injecting multiple filters, material table complains.
const originalError = console.error.bind(console.error)

global.console.error = (log) => {
  if (!log.toString().includes('Warning: Failed')) {
    originalError(log.toString())
  }
}

// Set API URL for Sippy
process.env.REACT_APP_API_URL = 'http://localhost:8080'

// Default PollyJS mode is replay, set to record to record
// new API calls.
if (!process.env.POLLY_MODE) {
  process.env.POLLY_MODE = 'replay'
}

export const setupDefaultPolly = () => {
  const context = setupPolly({
    mode: process.env.POLLY_MODE,
    adapters: [require('@pollyjs/adapter-node-http')],
    persister: require('@pollyjs/persister-fs'),
    persisterOptions: {
      fs: {
        recordingsDir: path.resolve(__dirname, '../__recordings__')
      }
    }
  })
  return context
}

export const expectLoadingPage = (wrapper) => expect(wrapper.find('p').contains('Loading...'))

jest.mock('react-chartjs-2', () => ({
  Bar: () => null,
  Chart: () => null,
  ChartComponent: () => null,
  Doughnut: () => null,
  Line: () => null,
  PolarArea: () => null
}))
