import { useEffect, useSyncExternalStore } from 'react'

import { apiGet, apiPost } from '@/api/client'
import { BrowseHistoryStore, VIEWED_MOVIES_PATH } from './browse-history-store'

export const browseHistory = new BrowseHistoryStore({
  fetchViewed: () => apiGet<string[]>(VIEWED_MOVIES_PATH),
  syncViewed: (ids: string[]) => apiPost<null>(VIEWED_MOVIES_PATH, { ids })
})

// useIsMovieViewed subscribes to the viewed state of one JavDB movie ID.
export function useIsMovieViewed(id?: string): boolean {
  useEffect(() => browseHistory.init(), [])
  return useSyncExternalStore(
    cb => browseHistory.subscribe(cb),
    () => browseHistory.isViewed(id)
  )
}

// useRecordMovieView marks the movie as viewed once per mounted ID.
export function useRecordMovieView(id: string): void {
  useEffect(() => {
    if (id) browseHistory.recordView(id)
  }, [id])
}
