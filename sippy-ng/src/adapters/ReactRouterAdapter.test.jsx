import '@testing-library/jest-dom'
import { act, render, screen } from '@testing-library/react'
import { BrowserRouter } from 'react-router'
import {
  QueryParamProvider,
  StringParam,
  useQueryParam,
} from 'use-query-params'
import { ReactRouterAdapter } from './ReactRouterAdapter'
import React from 'react'

function TestComponent() {
  const [value, setValue] = useQueryParam('foo', StringParam)
  return (
    <div>
      <span data-testid="value">{value ?? ''}</span>
      <button onClick={() => setValue('bar')}>Set</button>
    </div>
  )
}

function renderWithRouter(ui, { route = '/' } = {}) {
  window.history.pushState({}, '', route)
  return render(
    <BrowserRouter>
      <QueryParamProvider adapter={ReactRouterAdapter}>{ui}</QueryParamProvider>
    </BrowserRouter>
  )
}

describe('ReactRouterAdapter', () => {
  it('renders children via the adapter', () => {
    renderWithRouter(<TestComponent />)
    expect(screen.getByTestId('value')).toBeInTheDocument()
  })

  it('reads query params from the URL', () => {
    renderWithRouter(<TestComponent />, { route: '/?foo=hello' })
    expect(screen.getByTestId('value')).toHaveTextContent('hello')
  })

  it('updates query params on push', async () => {
    renderWithRouter(<TestComponent />)
    await act(async () => {
      screen.getByText('Set').click()
    })
    expect(window.location.search).toContain('foo=bar')
    expect(screen.getByTestId('value')).toHaveTextContent('bar')
  })
})
