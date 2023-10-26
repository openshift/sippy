/** @jest-environment setup-polly-jest/jest-environment-node */

import '@testing-library/jest-dom'
import { fetch as nodeFetch } from 'node-fetch'
import { setupPolly } from 'setup-polly-jest'
import Adapter from '@wojtekmaj/enzyme-adapter-react-17'
import Enzyme from 'enzyme'
import path from 'path'
import toDiffableHtml from 'diffable-html'

// https://github.com/mui-org/material-ui/issues/21293
export const withoutMuiID = (wrapper) =>
  toDiffableHtml(
    wrapper
      .html()
      .replace(/id="mui-[0-9]*"/g, '')
      .replace(/aria-labelledby="(mui-[0-9]* *)*"/g, '')
      .replace(/makeStyles-.*-[0-9]*/, '')
      .replace(/MuiBox-root-.*-[0-9]*/, '')
  )

Enzyme.configure({ adapter: new Adapter() })

// To stub out fetch(), we need to use node-fetch and this line:
global.fetch = nodeFetch

// See: https://github.com/vuejs/vue-test-utils/issues/974
global.requestAnimationFrame = (cb) => cb()

// Set API URL for Sippy
process.env.REACT_APP_API_URL = 'http://localhost:8080'

// Default PollyJS mode is replay, set to record to record
// new API calls.
if (!process.env.POLLY_MODE) {
  process.env.POLLY_MODE = 'replay'
}

export const setupDefaultPolly = () => {
  return setupPolly({
    mode: process.env.POLLY_MODE,
    adapters: [require('@pollyjs/adapter-node-http')],
    persister: require('@pollyjs/persister-fs'),
    persisterOptions: {
      fs: {
        recordingsDir: path.resolve(__dirname, '../__recordings__'),
      },
    },
  })
}

export const expectLoadingPage = (wrapper) =>
  expect(wrapper.find('p').contains('Loading...'))

jest.mock('react-chartjs-2', () => ({
  Bar: () => null,
  Chart: () => null,
  ChartComponent: () => null,
  Doughnut: () => null,
  Line: () => null,
  PolarArea: () => null,
}))
