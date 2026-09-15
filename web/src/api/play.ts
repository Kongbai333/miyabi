import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'

import { apiDelete, apiGet } from '@/api/client'
import type { LibraryFile } from '@/api/library'
import type { WatchHistoryScope, WatchResume } from '@/api/watch-history'

export type PlayFiles = {
  code: string
  title: string
  files: LibraryFile[]
  source: WatchHistoryScope
  resume?: WatchResume
}

export type PlaySource = {
  src: string
  type: 'application/x-mpegurl'
  label: string
}

type Playback = {
  id: string
  sources: PlaySource[]
}

const playQueryOptions = {
  retry: false,
  gcTime: 0,
  staleTime: Infinity,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false
} as const

export function usePlayFiles(movieID: number, openingID: string) {
  return useQuery({
    ...playQueryOptions,
    queryKey: ['play', 'files', movieID, openingID],
    queryFn: ({ signal }) => apiGet<PlayFiles>('/api/play/files', { movie_id: movieID }, signal)
  })
}

export function usePlayback(fileID: string, openingID: string) {
  const query = useQuery({
    ...playQueryOptions,
    queryKey: ['play', 'source', fileID, openingID],
    queryFn: ({ signal }) =>
      apiGet<Playback>(`/api/play/${encodeURIComponent(fileID)}`, undefined, signal)
  })
  const id = query.data?.id

  useEffect(() => {
    if (!id) return
    return () => {
      // The player aborts media requests on unmount; also release the server's URL registry.
      // Abandoned tabs and failed cleanup requests expire on the server.
      void apiDelete(`/api/play/${id}`).catch(() => {})
    }
  }, [id])

  return query
}
