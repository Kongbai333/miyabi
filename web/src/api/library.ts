import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError, apiGet, apiPost, apiPut } from '@/api/client'
import { panKeys, type PanAccountStatus } from '@/api/pan'
import { taskKeys, type LibrarySource, type ScanTask } from '@/api/tasks'
import type { WatchHistoryScope, WatchSession } from '@/api/watch-history'
import { notifyScanTask, notifyTaskError } from '@/features/tasks/task-toast'

export const LIBRARY_PAGE_SIZE = 20

export type LibraryEntity = { id?: string; name: string }

export type LibraryProgress = { position: number; duration: number }

export type LibraryMovie = {
  id: number
  code: string
  title: string
  javdb_id?: string
  cover?: string
  poster?: string
  fanart?: string
  release_date?: string
  duration: number
  size: number
  rating: number
  maker?: LibraryEntity
  series?: LibraryEntity
  director?: LibraryEntity
  actors: LibraryEntity[]
  tags: Array<{ id: number; javdb_id: string; name: string }>
  scrape_status: 'pending' | 'done' | 'failed'
  watched: boolean
  favorite_group_ids: number[]
  // Absent until the movie has actually been played.
  progress?: LibraryProgress
}

export type LibraryPage = {
  source?: LibrarySource
  movies: LibraryMovie[]
  total: number
  page: number
  has_more: boolean
}

export type LibraryFile = { id: string; name: string; path: string; size: number }

export type LibraryFilterOption = { id: string; name: string; count: number }

// The mounted library's own values, so the filter bar never offers a tag or an
// actor that no movie carries.
export type LibraryFilterOptions = {
  tags: LibraryFilterOption[]
  actors: LibraryFilterOption[]
  years: LibraryFilterOption[]
}

// LibrarySort orders the grid: newest scan first, or most recently opened first.
export type LibrarySort = 'added' | 'watched'

// Every dimension is a multi-select on the wire, so the API stays the same if
// the bar ever lets one dimension hold several values. The group tabs keep
// their own single-valued param and are merged in by useLibraryMovies.
export type LibraryFilter = {
  tagIds: number[]
  actorIds: string[]
  years: number[]
  watched: '' | 'yes' | 'no'
  sort: LibrarySort
}

export const libraryKeys = {
  all: ['library'] as const,
  movieLists: ['library', 'movies'] as const,
  movies: (page: number, group: number, filter: LibraryFilter) =>
    ['library', 'movies', page, group, filter] as const,
  filterOptions: ['library', 'filter-options'] as const
}

export function useLibraryMovies(page: number, group: number, filter: LibraryFilter) {
  return useQuery({
    queryKey: libraryKeys.movies(page, group, filter),
    queryFn: ({ signal }) =>
      apiGet<LibraryPage>(
        '/api/library/movies',
        {
          page,
          limit: LIBRARY_PAGE_SIZE,
          group_id: group > 0 ? [group] : undefined,
          tag_id: filter.tagIds,
          actor_id: filter.actorIds,
          year: filter.years,
          watched: filter.watched,
          sort: filter.sort
        },
        signal
      ),
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useLibraryFilterOptions() {
  return useQuery({
    queryKey: libraryKeys.filterOptions,
    queryFn: ({ signal }) =>
      apiGet<LibraryFilterOptions>('/api/library/filter-options', undefined, signal),
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useMarkMovieWatched() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, source }: { movieID: number; source: WatchHistoryScope }) =>
      apiPut<{ id: number; watched: boolean; history: WatchSession }>(
        `/api/library/movies/${movieID}/watched`,
        source,
        { signal: AbortSignal.timeout(10_000) }
      ),
    retry: (failures, error) =>
      failures < 2 && (!(error instanceof ApiError) || error.status >= 500),
    onSuccess: async ({ id, watched }) => {
      // An older list response must not restore "unwatched" after the write succeeds.
      await queryClient.cancelQueries({ queryKey: libraryKeys.movieLists })
      queryClient.setQueriesData<LibraryPage>({ queryKey: libraryKeys.movieLists }, page =>
        page
          ? {
              ...page,
              movies: page.movies.map(movie => (movie.id === id ? { ...movie, watched } : movie))
            }
          : page
      )
      // Refresh lists in the background so playback can start with the watch session.
      void queryClient.invalidateQueries({ queryKey: libraryKeys.all })
    },
    onError: () => {
      toast.error('观看记录保存失败', {
        id: 'library:watched-error',
        description: '播放仍可继续。请检查后端连接，稍后重新打开影片重试。'
      })
    }
  })
}

export function useStartLibraryScan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<ScanTask>('/api/library/scan'),
    onSuccess: task => {
      const account = queryClient.getQueryData<PanAccountStatus>(panKeys.account)
      if (
        account &&
        (!account.connected ||
          account.account?.id !== task.source.account_id ||
          account.directory?.id !== task.source.directory.id)
      )
        return queryClient.invalidateQueries({ queryKey: taskKeys.all })
      notifyScanTask(task)
      queryClient.setQueryData<ScanTask[]>(taskKeys.all, tasks => [
        task,
        ...(tasks ?? []).filter(item => item.id !== task.id)
      ])
      return queryClient.invalidateQueries({ queryKey: taskKeys.all })
    },
    onError: error => {
      notifyTaskError(
        'scan:submit-error',
        '无法创建扫描任务',
        error instanceof ApiError
          ? error.status === 401
            ? '115 登录已失效，请前往设置重新登录。'
            : error.message
          : '请检查后端服务和 115 连接后重试。'
      )
      if (error instanceof ApiError && (error.status === 401 || error.status === 400)) {
        void queryClient.invalidateQueries({ queryKey: panKeys.account })
        void queryClient.invalidateQueries({ queryKey: libraryKeys.all })
      }
    }
  })
}

// A library that failed to scrape before the video still existed still shows
// blank cards, and rescanning it would put every one of those movies to JavDB
// again. The covers appear as the queued stills finish, which is what
// NotifyLibraryChanged already refreshes.
export function useBackfillCovers() {
  return useMutation({
    mutationFn: () => apiPost<{ queued: number }>('/api/library/covers/backfill')
  })
}
