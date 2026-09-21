import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  EMPTY_LIBRARY_FILTER,
  hasLibraryFilter,
  libraryFilterOf,
  libraryFilterSearch,
  parseLibrarySearch
} from '../src/features/library/filter-params.ts'

test('an empty search reads as an unfiltered first page', () => {
  assert.deepEqual(parseLibrarySearch({}), {})
  assert.deepEqual(libraryFilterOf(parseLibrarySearch({})), EMPTY_LIBRARY_FILTER)
  assert.deepEqual(EMPTY_LIBRARY_FILTER, {
    tagIds: [],
    actorIds: [],
    years: [],
    watched: '',
    sort: 'added'
  })
  assert.equal(hasLibraryFilter(EMPTY_LIBRARY_FILTER), false)
})

test('every dimension survives a round trip through the URL', () => {
  const search = parseLibrarySearch({
    page: '3',
    group: '4',
    tag: '7',
    actor: 'actor-1',
    year: '2026',
    watched: 'no',
    sort: 'watched'
  })
  assert.deepEqual(search, {
    page: 3,
    group: 4,
    tag: 7,
    actor: 'actor-1',
    year: 2026,
    watched: 'no',
    sort: 'watched'
  })

  const filter = libraryFilterOf(search)
  assert.deepEqual(filter, {
    tagIds: [7],
    actorIds: ['actor-1'],
    years: [2026],
    watched: 'no',
    sort: 'watched'
  })
  assert.equal(hasLibraryFilter(filter), true)

  // The group tabs keep their own parameter, so it is threaded through by hand.
  // The page number is deliberately dropped: a new filter means a new list.
  assert.deepEqual(libraryFilterSearch(filter, 4), {
    group: 4,
    tag: 7,
    actor: 'actor-1',
    year: 2026,
    watched: 'no',
    sort: 'watched'
  })
})

test('the default order stays out of the URL', () => {
  assert.deepEqual(parseLibrarySearch({ sort: 'added' }), {})
  assert.deepEqual(libraryFilterSearch(EMPTY_LIBRARY_FILTER, 0), {})
  // The order is not a filter, so it cannot light up the clear button.
  assert.equal(hasLibraryFilter({ ...EMPTY_LIBRARY_FILTER, sort: 'watched' }), false)
})

test('a change to a filter, a group or the order drops the page number', () => {
  const search = parseLibrarySearch({ page: '9', tag: '7', year: '2026' })
  const filter = libraryFilterOf(search)
  assert.deepEqual(libraryFilterSearch({ ...filter, watched: 'yes' }, 0), {
    tag: 7,
    year: 2026,
    watched: 'yes'
  })
  assert.deepEqual(libraryFilterSearch(filter, 4), { group: 4, tag: 7, year: 2026 })
  assert.deepEqual(libraryFilterSearch({ ...filter, sort: 'watched' }, 0), {
    tag: 7,
    year: 2026,
    sort: 'watched'
  })
})

test('clearing one dimension leaves the others alone', () => {
  const filter = libraryFilterOf(parseLibrarySearch({ tag: '7', actor: 'actor-1' }))
  assert.deepEqual({ ...filter, tagIds: [] }, {
    ...EMPTY_LIBRARY_FILTER,
    actorIds: ['actor-1']
  })
  assert.equal(hasLibraryFilter({ ...filter, tagIds: [], actorIds: [] }), false)
})

test('a hand-edited or stale URL is refused instead of guessed', () => {
  for (const search of [
    { page: '0' },
    { page: '1.5' },
    { page: 'abc' },
    { group: '-1' },
    { tag: '0' },
    { year: '1899' },
    { year: '3000' },
    { watched: 'maybe' },
    { sort: 'random' },
    { tag: ['1', '2'] },
    { actor: ['a', 'b'] },
    { actor: 'x'.repeat(65) }
  ]) {
    assert.throws(() => parseLibrarySearch(search), `${JSON.stringify(search)} was accepted`)
  }
})

test('the group tabs may name group zero without narrowing the list', () => {
  assert.deepEqual(parseLibrarySearch({ group: '0' }), {})
  assert.deepEqual(parseLibrarySearch({ page: '1' }), {})
})
