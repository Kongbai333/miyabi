import assert from 'node:assert/strict'
import { test } from 'node:test'

import { BrowseHistoryStore } from '../src/api/browse-history-store.ts'

test('BrowseHistoryStore tracks viewed status and deduplicates entries', async t => {
  const synced = []
  const store = new BrowseHistoryStore({
    syncViewed: async ids => {
      synced.push(...ids)
    },
    storageKey: 'test:browse_history'
  })

  assert.equal(store.isViewed('test-id-1'), false)

  store.recordView('test-id-1', 'CODE-001')

  assert.equal(store.isViewed('test-id-1'), true)
  assert.equal(store.isViewed(undefined, 'CODE-001'), true)
  assert.equal(store.isViewed('test-id-2'), false)

  // Duplicate records are safe no-ops
  store.recordView('test-id-1', 'CODE-001')
  assert.equal(store.isViewed('test-id-1'), true)

  // Flush flushes pending queue
  await store.flush()
  assert.equal(store.getPendingCount(), 0)
  assert.deepEqual(synced, ['test-id-1', 'CODE-001'])
})

test('BrowseHistoryStore batch triggers flush at 50 items', async t => {
  const syncedBatches = []
  const store = new BrowseHistoryStore({
    syncViewed: async ids => {
      syncedBatches.push([...ids])
    },
    storageKey: 'test:browse_batch'
  })

  // Add 49 items -> should not flush immediately
  for (let i = 0; i < 49; i++) {
    store.recordView(`item-${i}`)
  }
  assert.equal(syncedBatches.length, 0)
  assert.equal(store.getPendingCount(), 49)

  // 50th item triggers immediate batch sync
  store.recordView('item-49')
  // Allow microtasks to resolve
  await new Promise(resolve => setTimeout(resolve, 10))
  assert.equal(syncedBatches.length, 1)
  assert.equal(syncedBatches[0].length, 50)
  assert.equal(store.getPendingCount(), 0)
})
