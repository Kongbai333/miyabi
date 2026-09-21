import { useSyncExternalStore } from 'react'

import { apiGet, apiPost } from '@/api/client'
import { BrowseHistoryStore } from './browse-history-store'

export const browseHistory = new BrowseHistoryStore({
  fetchViewed: () => apiGet<string[]>('/api/discover/viewed'),
  syncViewed: (ids: string[]) => apiPost<null>('/api/discover/viewed', { ids })
})

// Hook to check viewed status reactively
export function useIsMovieViewed(id?: string, code?: string): boolean {
  browseHistory.init()
  return useSyncExternalStore(
    cb => browseHistory.subscribe(cb),
    () => browseHistory.isViewed(id, code)
  )
}

// Hook to access recordView
export function useBrowseHistory() {
  browseHistory.init()
  return {
    isViewed: (id?: string, code?: string) => browseHistory.isViewed(id, code),
    recordView: (id: string, code?: string) => browseHistory.recordView(id, code)
  }
}
