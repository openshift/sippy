import '@testing-library/jest-dom'
import fetch from 'node-fetch'

// To stub out fetch(), we need to use node-fetch and this line:
global.fetch = fetch

// See: https://github.com/vuejs/vue-test-utils/issues/974
global.requestAnimationFrame = (cb) => cb()

// Set API URL for Sippy
import.meta.env.VITE_API_URL = 'http://localhost:8080'

vi.mock('react-chartjs-2', () => ({
  Bar: () => null,
  Chart: () => null,
  ChartComponent: () => null,
  Doughnut: () => null,
  Line: () => null,
  PolarArea: () => null,
}))
