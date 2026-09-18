import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError, apiDelete, apiGet, apiPost } from '@/api/client'

export type MonitorStatus = 'waiting' | 'added' | 'stale'

export type MonitorItem = {
  id: number
  movie_id: string
  code: string
  title: string
  cover: string
  release_date: string
  status: MonitorStatus
  hash?: string
  task_id?: number
  next_check_at?: string
  last_checked_at?: string
  checks: number
  error?: string
  created_at: string
  updated_at: string
}

export const monitorKeys = { all: ['monitors'] as const }

export function useMonitors(enabled = true) {
  return useQuery({
    queryKey: monitorKeys.all,
    queryFn: ({ signal }) => apiGet<MonitorItem[]>('/api/monitors', undefined, signal),
    enabled,
    staleTime: Infinity,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    retry: false
  })
}

export function useMonitor(movieID: string) {
  const monitors = useMonitors()
  return {
    monitor: monitors.data?.find(item => item.movie_id === movieID),
    isPending: monitors.isPending
  }
}

function describeError(error: unknown) {
  return error instanceof ApiError ? error.message : '请检查后端服务和网络后重试。'
}

export function useAddMonitor() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (movieID: string) => apiPost<MonitorItem>('/api/monitors', { movie_id: movieID }),
    retry: false,
    onSuccess: item => {
      queryClient.setQueryData<MonitorItem[]>(monitorKeys.all, items => [
        item,
        ...(items ?? []).filter(existing => existing.movie_id !== item.movie_id)
      ])
      toast.success(item.code, { description: '已加入监控，出现磁力后会自动加入 115。' })
      void queryClient.invalidateQueries({ queryKey: monitorKeys.all })
    },
    onError: error => toast.error('加入监控失败', { description: describeError(error) })
  })
}

export function useRemoveMonitor() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (movieID: string) =>
      apiDelete<null>(`/api/monitors/${encodeURIComponent(movieID)}`),
    retry: false,
    onSuccess: (_, movieID) => {
      queryClient.setQueryData<MonitorItem[]>(monitorKeys.all, items =>
        (items ?? []).filter(item => item.movie_id !== movieID)
      )
      void queryClient.invalidateQueries({ queryKey: monitorKeys.all })
    },
    onError: error => toast.error('取消监控失败', { description: describeError(error) })
  })
}

export function useRetryMonitor() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (movieID: string) =>
      apiPost<MonitorItem>(`/api/monitors/${encodeURIComponent(movieID)}/retry`),
    retry: false,
    onSuccess: item => {
      queryClient.setQueryData<MonitorItem[]>(monitorKeys.all, items =>
        (items ?? []).map(existing => (existing.movie_id === item.movie_id ? item : existing))
      )
      toast.success(item.code, { description: '已重新开始监控。' })
      void queryClient.invalidateQueries({ queryKey: monitorKeys.all })
    },
    onError: error => toast.error('重新监控失败', { description: describeError(error) })
  })
}
