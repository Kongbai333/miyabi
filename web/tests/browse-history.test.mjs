import assert from 'node:assert/strict'
import { test } from 'node:test'

import { BrowseHistoryStore } from '../src/api/browse-history-store.ts'

test('BrowseHistoryStore tracks viewed IDs and deduplicates entries', async () => {
  const synced = []
  const store = new BrowseHistoryStore({
    syncViewed: async ids => {
      synced.push(...ids)
    },
    storageKey: 'test:browse_history'
  })

  assert.equal(store.isViewed('test-id-1'), false)
  assert.equal(store.isViewed(undefined), false)

  store.recordView('test-id-1')
  store.recordView(' test-id-1 ')
  store.recordView('   ')

  assert.equal(store.isViewed('test-id-1'), true)
  assert.equal(store.isViewed('test-id-2'), false)
  assert.equal(store.getPendingCount(), 1)

  await store.flush()
  assert.equal(store.getPendingCount(), 0)
  assert.deepEqual(synced, ['test-id-1'])
})

test('BrowseHistoryStore keeps pending entries when sync fails', async () => {
  let fail = true
  const store = new BrowseHistoryStore({
    syncViewed: async () => {
      if (fail) throw new Error('offline')
    },
    storageKey: 'test:browse_retry'
  })
  store.recordView('retry-id')
  await store.flush()
  assert.equal(store.getPendingCount(), 1)
  fail = false
  await store.flush()
  assert.equal(store.getPendingCount(), 0)
})

test('BrowseHistoryStore batch triggers flush at 50 items', async () => {
  const syncedBatches = []
  const store = new BrowseHistoryStore({
    syncViewed: async ids => {
      syncedBatches.push([...ids])
    },
    storageKey: 'test:browse_batch'
  })

  for (let i = 0; i < 49; i++) store.recordView(`item-${i}`)
  assert.equal(syncedBatches.length, 0)
  assert.equal(store.getPendingCount(), 49)

  store.recordView('item-49')
  await new Promise(resolve => setTimeout(resolve, 10))
  assert.equal(syncedBatches.length, 1)
  assert.equal(syncedBatches[0].length, 50)
  assert.equal(store.getPendingCount(), 0)
})
