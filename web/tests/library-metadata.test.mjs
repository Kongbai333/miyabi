import assert from 'node:assert/strict'
import { test } from 'node:test'

import { libraryMovieMetadata, libraryMovieSummary } from '../src/features/library/movie-metadata.ts'

test('library hover links use scraped JavDB IDs instead of local database IDs', () => {
  const metadata = libraryMovieMetadata({
    maker: { id: 'maker-remote', name: '厂牌' },
    series: { id: 'series-remote', name: '系列' },
    director: { id: 'director-remote', name: '导演' },
    actors: [{ id: 'actor-remote', name: '演员' }],
    tags: [{ id: 42, javdb_id: 'tag-remote', name: '标签' }]
  })
  assert.deepEqual(metadata, {
    maker: { id: 'maker-remote', name: '厂牌' },
    series: { id: 'series-remote', name: '系列' },
    director: { id: 'director-remote', name: '导演' },
    actors: [{ id: 'actor-remote', name: '演员' }],
    tags: [{ id: 'tag-remote', name: '标签' }]
  })
})

test('incomplete local metadata keeps names without inventing searchable IDs', () => {
  const metadata = libraryMovieMetadata({
    maker: { name: '本地厂牌' },
    tags: [{ id: 42, name: '旧标签' }]
  })
  assert.deepEqual(metadata.maker, { name: '本地厂牌' })
  assert.deepEqual(metadata.actors, [])
  assert.deepEqual(metadata.tags, [{ id: undefined, name: '旧标签' }])
  assert.equal(metadata.zone, undefined)
})

test('the card summary shows duration and video size, in megabytes', () => {
  // Not 2 << 30: JS bitwise operators are signed 32-bit and would overflow.
  assert.equal(libraryMovieSummary({ duration: 125, size: 2048 * 1024 * 1024 }), '125 分钟 · 2,048 MB')
  assert.equal(libraryMovieSummary({ duration: 0, size: 1024 * 1024 }), '1 MB')
})

test('a movie with neither duration nor size adds no summary line', () => {
  assert.equal(libraryMovieSummary({ duration: 0, size: 0 }), '')
})
