import assert from 'node:assert/strict'
import { test } from 'node:test'

import { getPageNumbers } from '../src/lib/pagination.ts'

test('getPageNumbers returns all pages when totalPages <= 7', () => {
  assert.deepEqual(getPageNumbers(1, 1), [1])
  assert.deepEqual(getPageNumbers(1, 5), [1, 2, 3, 4, 5])
  assert.deepEqual(getPageNumbers(4, 7), [1, 2, 3, 4, 5, 6, 7])
})

test('getPageNumbers returns right ellipsis when page is near start', () => {
  assert.deepEqual(getPageNumbers(1, 10), [1, 2, 3, 4, 5, 'ellipsis-right', 10])
  assert.deepEqual(getPageNumbers(4, 10), [1, 2, 3, 4, 5, 'ellipsis-right', 10])
})

test('getPageNumbers returns left ellipsis when page is near end', () => {
  assert.deepEqual(getPageNumbers(7, 10), [1, 'ellipsis-left', 6, 7, 8, 9, 10])
  assert.deepEqual(getPageNumbers(10, 10), [1, 'ellipsis-left', 6, 7, 8, 9, 10])
})

test('getPageNumbers returns both ellipses when page is in the middle', () => {
  assert.deepEqual(getPageNumbers(5, 10), [1, 'ellipsis-left', 4, 5, 6, 'ellipsis-right', 10])
  assert.deepEqual(getPageNumbers(6, 10), [1, 'ellipsis-left', 5, 6, 7, 'ellipsis-right', 10])
})
