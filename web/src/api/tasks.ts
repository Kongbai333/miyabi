import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPost } from '@/api/client'
import type { PanDirectory } from '@/api/pan'
export type LibrarySource = {
  account_id: string
  directory: PanDirectory
}

export type TaskRevisions = { library: number; offline: number; history: number; monitor: number }

export type ScanTask = {
  id: number
  type: 'scan'
  status: 'queued' | 'running' | 'done' | 'failed'
  progress: number
  error?: string
  created_at: string
  updated_at: string
  source: LibrarySource
  offline_task_id?: number
  scan: {
    stage: 'queued' | 'scanning' | 'reconciling' | 'scraping' | 'artwork' | 'done'
    current_path: string
    directories_discovered: number
    directories_scanned: number
    files_scanned: number
    video_files: number
    matched_files: number
    unmatched_files: number
    movies: number
    removed_files: number
    removed_movies: number
    metadata_total: number
    metadata_completed: number
  }
}

export type TaskType = 'scan' | 'scrape' | 'cover' | 'frame'

// A queue is what the pools hold right now, not what they have finished.
export type TaskQueue = {
  type: TaskType
  queued: number
  running: number
  paused: boolean
}

export const taskKeys = { all: ['tasks'] as const, queues: ['tasks', 'queues'] as const }

export function useTasks() {
  return useQuery({
    queryKey: taskKeys.all,
    queryFn: ({ signal }) => apiGet<ScanTask[]>('/api/tasks', undefined, signal),
    staleTime: Infinity,
    refetchOnMount: 'always',
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function isTaskActive(task: ScanTask) {
  return task.status === 'queued' || task.status === 'running'
}

// The queues arrive with every stream event and from their own route on mount,
// so a page that opens while the stream is down still shows them.
export function useTaskQueues() {
  return useQuery({
    queryKey: taskKeys.queues,
    queryFn: ({ signal }) => apiGet<TaskQueue[]>('/api/tasks/queues', undefined, signal),
    staleTime: Infinity,
    refetchOnMount: 'always',
    retry: false,
    refetchOnWindowFocus: false
  })
}

// Pausing and resuming answer with the queues they produced, so the page
// redraws from the response instead of waiting for the next event.
function useTaskToggle(toggle: (types: TaskType[]) => Promise<TaskQueue[]>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: toggle,
    retry: false,
    onSuccess: queues => queryClient.setQueryData(taskKeys.queues, queues)
  })
}

export function usePauseTasks() {
  return useTaskToggle(types => apiPost<TaskQueue[]>('/api/tasks/pause', { types }))
}

export function useResumeTasks() {
  return useTaskToggle(types => apiPost<TaskQueue[]>('/api/tasks/resume', { types }))
}

// Cancelling only reports how many tasks were dropped, so the queues are read
// again after it.
export function useCancelQueuedTasks() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (types: TaskType[]) => {
      const query = new URLSearchParams()
      for (const type of types) query.append('type', type)
      return apiDelete<{ cancelled: number }>(`/api/tasks/queued?${query.toString()}`)
    },
    retry: false,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: taskKeys.queues })
  })
}
