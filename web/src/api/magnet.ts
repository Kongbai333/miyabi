import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPost, apiPut } from '@/api/client'

// The magnet server is a model context protocol endpoint this app talks to
// through the backend, so every page call below is an app route rather than a
// direct request.

export type MagnetSettings = { url: string; token: string }

export type MagnetTestResult = { tools: string[] }

export type MagnetItem = {
  name: string
  magnet_url: string
  link: string
  size: string
  date: string
  score: number
}

export type MagnetSearchResult = {
  query: string
  source: string
  count: number
  local_best_score: number
  items: MagnetItem[]
}

export type MagnetScreenshot = { screenshot: string; time: number }

export type MagnetPreview = {
  magnet_link: string
  size: number
  data: {
    count: number
    error: string
    name: string
    file_type: string
    type: string
    size: number
    screenshots: MagnetScreenshot[]
  }
}

export type MagnetFile = {
  id: string
  name: string
  file_size_bytes: number
  is_dir: boolean
  meta: { icon: string; mime_type: string; hash: string; url_tag: string }
  sub_files: MagnetFile[]
}

export type MagnetFiles = {
  magnet_link: string
  list_id: string
  file_count: number
  total_size: number
  files: MagnetFile[]
}

export type MagnetCollection = {
  key: string
  label: string
  deletable: boolean
  is_default: boolean
}

export type MagnetCollectionItem = {
  name: string
  tagsText: string[]
  magnet_url: string
  added_at: number
  collection: string
}

export type MagnetCollectionDetail = {
  collection: MagnetCollection
  count: number
  items: MagnetCollectionItem[]
}

export type MagnetShare = { code: string; rss_url: string; share_url: string }

export type MagnetShareDetail = MagnetShare & {
  label: string
  count: number
  items: MagnetCollectionItem[]
  owner_id: number
  updated_at: string
}

export const magnetKeys = {
  all: ['magnet'] as const,
  settings: ['magnet', 'settings'] as const,
  collections: ['magnet', 'collections'] as const,
  collection: (key: string) => ['magnet', 'collections', key] as const,
  preview: (link: string) => ['magnet', 'preview', link] as const,
  files: (link: string) => ['magnet', 'files', link] as const,
  share: (code: string) => ['magnet', 'share', code] as const
}

export function useMagnetSettings() {
  return useQuery({
    queryKey: magnetKeys.settings,
    queryFn: ({ signal }) => apiGet<MagnetSettings>('/api/magnet/settings', undefined, signal),
    staleTime: 15_000,
    retry: false,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false
  })
}

export function useUpdateMagnetSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (settings: MagnetSettings) =>
      apiPut<MagnetSettings>('/api/magnet/settings', settings),
    onSuccess: settings => queryClient.setQueryData(magnetKeys.settings, settings)
  })
}

export function useTestMagnet() {
  return useMutation({ mutationFn: () => apiPost<MagnetTestResult>('/api/magnet/test') })
}

// Searching walks the server's own database and may fall back to crawling, so
// it is an explicit action rather than a query that refires on render.
export function useMagnetSearch() {
  return useMutation({
    mutationFn: ({ query, limit }: { query: string; limit?: number }) =>
      apiPost<MagnetSearchResult>('/api/magnet/search', { query, limit })
  })
}

// A magnet's preview never changes, so it is fetched once per link.
export function useMagnetPreview(link: string) {
  return useQuery({
    queryKey: magnetKeys.preview(link),
    queryFn: () => apiPost<MagnetPreview>('/api/magnet/preview', { magnet_link: link }),
    enabled: link !== '',
    staleTime: Infinity,
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useMagnetFiles(link: string) {
  return useQuery({
    queryKey: magnetKeys.files(link),
    queryFn: () => apiPost<MagnetFiles>('/api/magnet/files', { magnet_link: link }),
    enabled: link !== '',
    staleTime: Infinity,
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useMagnetCollections() {
  return useQuery({
    queryKey: magnetKeys.collections,
    queryFn: ({ signal }) =>
      apiGet<MagnetCollection[]>('/api/magnet/collections', undefined, signal),
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useMagnetCollection(key: string) {
  return useQuery({
    queryKey: magnetKeys.collection(key),
    queryFn: ({ signal }) =>
      apiGet<MagnetCollectionDetail>(
        `/api/magnet/collections/${encodeURIComponent(key)}`,
        undefined,
        signal
      ),
    enabled: key !== '',
    staleTime: 15_000,
    retry: false,
    refetchOnWindowFocus: false
  })
}

// Every collection mutation changes the list, so one helper refreshes it.
function useCollectionMutation<TInput, TResult>(mutate: (input: TInput) => Promise<TResult>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: mutate,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: magnetKeys.all })
  })
}

export function useCreateMagnetCollection() {
  return useCollectionMutation((label: string) => apiPost('/api/magnet/collections', { label }))
}

export function useRenameMagnetCollection() {
  return useCollectionMutation(({ key, label }: { key: string; label: string }) =>
    apiPut(`/api/magnet/collections/${encodeURIComponent(key)}`, { label })
  )
}

export function useDeleteMagnetCollection() {
  return useCollectionMutation((key: string) =>
    apiDelete(`/api/magnet/collections/${encodeURIComponent(key)}`)
  )
}

export function useEnableMagnetShare() {
  return useCollectionMutation((key: string) =>
    apiPost<MagnetShare>(`/api/magnet/collections/${encodeURIComponent(key)}/share`)
  )
}

export function useDisableMagnetShare() {
  return useCollectionMutation((key: string) =>
    apiDelete(`/api/magnet/collections/${encodeURIComponent(key)}/share`)
  )
}

export function useImportMagnetShare() {
  return useCollectionMutation(({ code, label }: { code: string; label?: string }) =>
    apiPost(`/api/magnet/shares/import`, { code, label })
  )
}

export function useMagnetShareDetail(code: string) {
  return useQuery({
    queryKey: magnetKeys.share(code),
    queryFn: ({ signal }) =>
      apiGet<MagnetShareDetail>(
        `/api/magnet/shares/${encodeURIComponent(code)}`,
        undefined,
        signal
      ),
    enabled: code !== '',
    retry: false,
    refetchOnWindowFocus: false
  })
}
