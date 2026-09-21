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
  assert.equal(hasLibraryFilter(EMPTY_LIBRARY_FILTER), false)
})

test('every dimension survives a round trip through the URL', () => {
  const search = parseLibrarySearch({
    group: '4',
    tag: '7',
    actor: 'actor-1',
    series: 'series-1',
    maker: 'maker-1',
    director: 'director-1',
    year: '2026',
    watched: 'no'
  })
  assert.deepEqual(search, {
    group: 4,
    tag: 7,
    actor: 'actor-1',
    series: 'series-1',
    maker: 'maker-1',
    director: 'director-1',
    year: 2026,
    watched: 'no'
  })

  const filter = libraryFilterOf(search)
  assert.deepEqual(filter, {
    tagIds: [7],
    actorIds: ['actor-1'],
    seriesIds: ['series-1'],
    makerIds: ['maker-1'],
    directorIds: ['director-1'],
    years: [2026],
    watched: 'no'
  })
  assert.equal(hasLibraryFilter(filter), true)

  // The group tabs keep their own parameter, so it is threaded through by hand.
  assert.deepEqual(libraryFilterSearch(filter, 4), search)
})

test('a change to a filter or a group drops the page number', () => {
  const search = parseLibrarySearch({ page: '9', tag: '7', year: '2026' })
  const filter = libraryFilterOf(search)
  assert.deepEqual(libraryFilterSearch({ ...filter, watched: 'yes' }, 0), {
    tag: 7,
    year: 2026,
    watched: 'yes'
  })
  assert.deepEqual(libraryFilterSearch(filter, 4), { group: 4, tag: 7, year: 2026 })
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
