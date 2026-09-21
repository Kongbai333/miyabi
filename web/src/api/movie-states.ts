import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiPost, apiPut } from '@/api/client'
import {
  createMovieStateLoader,
  movieStateKeys,
  movieStateOptions,
  type MovieIdentity,
  type MovieStateResult
} from '@/api/movie-state-cache'

const loadMovieState = createMovieStateLoader(movies =>
  apiPost<MovieStateResult[]>('/api/discover/movie-states', { movies })
)

export function useMovieState(movie?: MovieIdentity) {
  return useQuery(movieStateOptions(loadMovieState, movie)).data
}

// Opening a detail page is what marks a movie as viewed. The write is
// best-effort: a failure must not interrupt reading the detail page. Leaving
// the page is the common case, so the request outlives the document instead of
// being cancelled with it.
export function useMarkMovieViewed() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiPut<{ id: string; viewed: boolean }>(
        `/api/discover/movies/${encodeURIComponent(id)}/viewed`,
        {},
        { keepalive: true, signal: AbortSignal.timeout(10_000) }
      ),
    onSuccess: (_result, id) =>
      queryClient.invalidateQueries({ queryKey: movieStateKeys.movie(id) })
  })
}
