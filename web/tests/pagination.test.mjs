import assert from 'node:assert/strict'
import { test } from 'node:test'

import { pageNumbers } from '../src/lib/pagination.ts'

test('a short list shows every page', () => {
  assert.deepEqual(pageNumbers(1, 1), [1])
  assert.deepEqual(pageNumbers(2, 3), [1, 2, 3])
  assert.deepEqual(pageNumbers(1, 5), [1, 2, 3, 4, 5])
})

test('a list the window almost covers is shown whole', () => {
  // One page on either side of the window is all an ellipsis would hide.
  assert.deepEqual(pageNumbers(3, 5), [1, 2, 3, 4, 5])
  assert.deepEqual(pageNumbers(3, 6), [2, 3, 4])
})

test('the window follows the current page without running past either end', () => {
  assert.deepEqual(pageNumbers(1, 20), [1, 2, 3])
  assert.deepEqual(pageNumbers(2, 20), [1, 2, 3])
  assert.deepEqual(pageNumbers(10, 20), [9, 10, 11])
  assert.deepEqual(pageNumbers(19, 20), [18, 19, 20])
  assert.deepEqual(pageNumbers(20, 20), [18, 19, 20])
})

test('a wider window is honoured where the list is long enough for it', () => {
  assert.deepEqual(pageNumbers(10, 20, 5), [8, 9, 10, 11, 12])
  assert.deepEqual(pageNumbers(1, 20, 5), [1, 2, 3, 4, 5])
})

test('an unknown or empty list renders no page buttons', () => {
  assert.deepEqual(pageNumbers(1, 0), [])
  assert.deepEqual(pageNumbers(1, -3), [])
  assert.deepEqual(pageNumbers(1, 1.5), [])
})
