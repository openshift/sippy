import { render } from '@testing-library/react'
//import HelloWorld from './HelloWorld'
import { dateEndFormat, dateFormat, formatLongDate } from './CompReadyUtils'
import React from 'react'

test('parses a query param start time without timezone', () => {
  let result = formatLongDate('2024-09-05 00:00:00', dateFormat)
  expect(result).toBe('2024-09-05 00:00:00')
})

test('parses a query param start time eod without timezone', () => {
  let result = formatLongDate('2024-09-05 23:59:59', dateFormat)
  expect(result).toBe('2024-09-05 00:00:00')
})

test('parses a query param end time without timezone', () => {
  let result = formatLongDate('2024-09-05 23:59:59', dateEndFormat)
  expect(result).toBe('2024-09-05 23:59:59')
})

test('parses an ISO8601 start time', () => {
  let result = formatLongDate('2024-08-06T00:00:00Z', dateFormat)
  expect(result).toBe('2024-08-06 00:00:00')
})

test('parses an ISO8601 end time mid-day', () => {
  let result = formatLongDate('2024-08-06T08:00:00Z', dateEndFormat)
  expect(result).toBe('2024-08-06 23:59:59')
})

test('parses an ISO8601 end time', () => {
  let result = formatLongDate('2024-08-06T23:59:59Z', dateEndFormat)
  expect(result).toBe('2024-08-06 23:59:59')
})
