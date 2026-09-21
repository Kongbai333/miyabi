import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { ApiError, apiGet, apiPost } from '@/api/client'

// The session lives in an HttpOnly cookie the page cannot read, so the gate
// asks the server whether the current cookie is still valid.
export function useAccessGateSession(enabled: boolean) {
  return useQuery({
    queryKey: ['auth', 'session'],
    queryFn: async ({ signal }) => {
      try {
        return await apiGet<{ authenticated: boolean; enabled: boolean }>(
          '/api/auth/session',
          undefined,
          signal
        )
      } catch (error) {
        // 401 is the expected "not signed in" answer, not a failed request.
        if (error instanceof ApiError && error.status === 401) {
          return { authenticated: false, enabled: true }
        }
        throw error
      }
    },
    enabled,
    staleTime: Infinity,
    retry: false
  })
}

export function useAccessGateLogin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (password: string) =>
      apiPost<{ success: boolean }>('/api/auth/login', { password }),
    onSuccess: () =>
      queryClient.setQueryData(['auth', 'session'], { authenticated: true, enabled: true })
  })
}

export function useAccessGateLogout() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<{ success: boolean }>('/api/auth/logout'),
    onSuccess: () => {
      // Drop cached catalogue data so the next sign-in does not flash it.
      queryClient.clear()
    }
  })
}
