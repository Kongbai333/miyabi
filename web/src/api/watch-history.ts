import { useMutation, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiPut } from '@/api/client'
import { libraryKeys } from '@/api/library'
import { watchSessions } from '@/features/player/watch-progress'

export type WatchSession = {
  id: number
  session_id: string
  file_id: string
  position: number
  duration: number
}

export type WatchResume = Omit<WatchSession, 'session_id'>
export type WatchHistoryScope = { account_id: string; directory_id: string }

export type WatchProgress = {
  session_id: string
  file_id: string
  position: number
  duration: number
  version: number
}

export function saveWatchProgress(id: number, progress: WatchProgress, keepalive: boolean) {
  return apiPut<null>(`/api/library/history/${id}/progress`, progress, {
    keepalive,
    signal: AbortSignal.timeout(10_000)
  })
}

// Clearing history drops every saved position, which is what takes the progress
// bars off the grid, so the library list is what has to be refetched.
export function useClearWatchHistory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (scope: WatchHistoryScope) =>
      apiDelete<{ removed: number }>(`/api/library/history?${new URLSearchParams(scope)}`),
    onSuccess: async (_, scope) => {
      watchSessions.clear(scope)
      await queryClient.cancelQueries({ queryKey: libraryKeys.all })
      return queryClient.invalidateQueries({ queryKey: libraryKeys.all })
    }
  })
}
