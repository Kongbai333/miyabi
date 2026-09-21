const STORAGE_KEY = 'miyabi:viewed_movies'
const BATCH_SIZE = 50
const FLUSH_DELAY_MS = 10_000
const MAX_LOCAL_IDS = 5_000

export type BrowseHistoryOptions = {
  fetchViewed?: () => Promise<string[]>
  syncViewed?: (ids: string[]) => Promise<unknown>
  storageKey?: string
}

type PersistedState = {
  ids: string[]
  pending: string[]
}

function loadPersistedState(key: string): PersistedState {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ids: [], pending: [] }
  }
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return { ids: [], pending: [] }
    const parsed = JSON.parse(raw) as Partial<PersistedState>
    return {
      ids: Array.isArray(parsed.ids) ? parsed.ids : [],
      pending: Array.isArray(parsed.pending) ? parsed.pending : []
    }
  } catch {
    return { ids: [], pending: [] }
  }
}

function savePersistedState(key: string, state: PersistedState): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.setItem(key, JSON.stringify(state))
  } catch {
    // Ignore storage quota errors
  }
}

export class BrowseHistoryStore {
  private viewedSet: Set<string>
  private pendingList: string[]
  private listeners = new Set<() => void>()
  private flushTimer: ReturnType<typeof setTimeout> | null = null
  private isFlushing = false
  private isInitialized = false
  private storageKey: string
  private fetchViewed?: () => Promise<string[]>
  private syncViewed?: (ids: string[]) => Promise<unknown>

  constructor(options: BrowseHistoryOptions = {}) {
    this.storageKey = options.storageKey ?? STORAGE_KEY
    this.fetchViewed = options.fetchViewed
    this.syncViewed = options.syncViewed

    const saved = loadPersistedState(this.storageKey)
    this.viewedSet = new Set(saved.ids)
    this.pendingList = [...saved.pending]

    if (typeof window !== 'undefined') {
      window.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') {
          void this.flush()
        }
      })
      window.addEventListener('pagehide', () => {
        this.flushKeepalive()
      })
    }
  }

  init(): void {
    if (this.isInitialized || typeof window === 'undefined') return
    this.isInitialized = true

    void this.syncFromServer()
  }

  private async syncFromServer(): Promise<void> {
    if (!this.fetchViewed) return
    try {
      const serverIDs = await this.fetchViewed()
      if (Array.isArray(serverIDs) && serverIDs.length > 0) {
        let changed = false
        for (const id of serverIDs) {
          if (!this.viewedSet.has(id)) {
            this.viewedSet.add(id)
            changed = true
          }
        }
        if (changed) {
          this.persist()
          this.emit()
        }
      }
    } catch {
      // Ignore network errors on initial sync; local cache remains active
    }

    if (this.pendingList.length > 0) {
      void this.flush()
    }
  }

  isViewed(id?: string, code?: string): boolean {
    if (id && this.viewedSet.has(id)) return true
    if (code && this.viewedSet.has(code)) return true
    return false
  }

  recordView(id: string, code?: string): void {
    const cleanID = id.trim()
    const cleanCode = code?.trim()
    if (!cleanID) return

    let changed = false
    if (!this.viewedSet.has(cleanID)) {
      this.viewedSet.add(cleanID)
      this.pendingList.push(cleanID)
      changed = true
    }
    if (cleanCode && !this.viewedSet.has(cleanCode)) {
      this.viewedSet.add(cleanCode)
      this.pendingList.push(cleanCode)
      changed = true
    }

    if (!changed) return

    this.persist()
    this.emit()

    if (this.pendingList.length >= BATCH_SIZE) {
      if (this.flushTimer) {
        clearTimeout(this.flushTimer)
        this.flushTimer = null
      }
      void this.flush()
    } else {
      this.scheduleDebouncedFlush()
    }
  }

  private scheduleDebouncedFlush(): void {
    if (this.flushTimer) clearTimeout(this.flushTimer)
    this.flushTimer = setTimeout(() => {
      this.flushTimer = null
      void this.flush()
    }, FLUSH_DELAY_MS)
    if (typeof this.flushTimer === 'object' && this.flushTimer !== null && 'unref' in this.flushTimer) {
      (this.flushTimer as { unref: () => void }).unref()
    }
  }

  async flush(): Promise<void> {
    if (this.isFlushing || this.pendingList.length === 0 || !this.syncViewed) return
    this.isFlushing = true

    const toSync = [...this.pendingList]
    try {
      await this.syncViewed(toSync)
      const syncedSet = new Set(toSync)
      this.pendingList = this.pendingList.filter(item => !syncedSet.has(item))
      this.persist()
    } catch {
      // Keep in pendingList for next retry
    } finally {
      this.isFlushing = false
    }
  }

  private flushKeepalive(): void {
    if (this.pendingList.length === 0 || typeof window === 'undefined') return
    const ids = [...this.pendingList]
    try {
      const body = JSON.stringify({ ids })
      if (navigator.sendBeacon) {
        const blob = new Blob([body], { type: 'application/json' })
        navigator.sendBeacon('/api/discover/viewed', blob)
      } else {
        void fetch('/api/discover/viewed', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body,
          keepalive: true
        })
      }
    } catch {
      // Ignore beacon failures during teardown
    }
  }

  private persist(): void {
    let ids = Array.from(this.viewedSet)
    if (ids.length > MAX_LOCAL_IDS) {
      ids = ids.slice(ids.length - MAX_LOCAL_IDS)
      this.viewedSet = new Set(ids)
    }
    savePersistedState(this.storageKey, {
      ids,
      pending: this.pendingList
    })
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  private emit(): void {
    for (const listener of this.listeners) {
      listener()
    }
  }

  getPendingCount(): number {
    return this.pendingList.length
  }
}
