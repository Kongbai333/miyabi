import assert from 'node:assert/strict'
import { test } from 'node:test'

import { pageNumbers } from '../src/lib/pagination.ts'

test('a short list shows every page', () => {
  assert.deepEqual(pageNumbers(1, 1), [1])
  assert.deepEqual(pageNumbers(2, 3), [1, 2, 3])
  assert.deepEqual(pageNumbers(1, 5), [1, 2, 3, 4, 5])
})

test('the window follows the current page without running past either end', () => {
  assert.deepEqual(pageNumbers(1, 20), [1, 2, 3, 4, 5])
  assert.deepEqual(pageNumbers(2, 20), [1, 2, 3, 4, 5])
  assert.deepEqual(pageNumbers(10, 20), [8, 9, 10, 11, 12])
  assert.deepEqual(pageNumbers(19, 20), [16, 17, 18, 19, 20])
  assert.deepEqual(pageNumbers(20, 20), [16, 17, 18, 19, 20])
})

test('an unknown or empty list renders no page buttons', () => {
  assert.deepEqual(pageNumbers(1, 0), [])
  assert.deepEqual(pageNumbers(1, -3), [])
  assert.deepEqual(pageNumbers(1, 1.5), [])
})
