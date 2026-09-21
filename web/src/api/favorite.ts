import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPost, apiPut } from '@/api/client'
import { libraryKeys, type LibraryPage } from '@/api/library'

export type FavoriteGroup = {
  id: number
  name: string
  count: number
}

export const favoriteKeys = {
  all: ['library', 'favorite-groups'] as const,
  groups: ['library', 'favorite-groups', 'list'] as const
}

export function useFavoriteGroups() {
  return useQuery({
    queryKey: favoriteKeys.groups,
    queryFn: ({ signal }) =>
      apiGet<FavoriteGroup[]>('/api/library/favorite-groups', undefined, signal),
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useCreateFavoriteGroup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => apiPost<FavoriteGroup>('/api/library/favorite-groups', { name }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: favoriteKeys.all })
  })
}

export function useRenameFavoriteGroup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) =>
      apiPut<FavoriteGroup>(`/api/library/favorite-groups/${id}`, { name }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: favoriteKeys.all })
  })
}

export function useDeleteFavoriteGroup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => apiDelete<null>(`/api/library/favorite-groups/${id}`),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: favoriteKeys.all })
      // Deleting a group drops its stars, so the cards must re-read them.
      return queryClient.invalidateQueries({ queryKey: libraryKeys.movieLists })
    }
  })
}

// The group list replaces the star set, so an empty array clears the star.
export function useSetFavorite() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, groupIDs }: { movieID: number; groupIDs: number[] }) =>
      apiPut<{ id: number; group_ids: number[] }>(
        `/api/library/movies/${movieID}/favorite`,
        { group_ids: groupIDs },
        { signal: AbortSignal.timeout(10_000) }
      ),
    onSuccess: async ({ id, group_ids }) => {
      // A slower list response must not restore the previous star set.
      await queryClient.cancelQueries({ queryKey: libraryKeys.movieLists })
      queryClient.setQueriesData<LibraryPage>({ queryKey: libraryKeys.movieLists }, page =>
        page
          ? {
              ...page,
              movies: page.movies.map(movie =>
                movie.id === id ? { ...movie, favorite_group_ids: group_ids } : movie
              )
            }
          : page
      )
      void queryClient.invalidateQueries({ queryKey: favoriteKeys.all })
    }
  })
}
