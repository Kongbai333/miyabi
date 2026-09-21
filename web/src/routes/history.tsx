import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { WatchHistoryPage } from '@/features/history/page'

type HistorySearch = { page?: number; group?: number }

export const Route = createFileRoute('/history')({
  validateSearch: (search: Record<string, unknown>): HistorySearch => {
    const page = Number(search.page ?? 1)
    if (!Number.isInteger(page) || page < 1 || page > 100_000_000) {
      throw new Error('观看历史页码无效')
    }
    const group = Number(search.group ?? 0)
    if (!Number.isInteger(group) || group < 0) throw new Error('观看历史分组无效')
    return { ...(page > 1 && { page }), ...(group > 0 && { group }) }
  },
  component: HistoryRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="观看历史链接无效" />
    </AppPage>
  )
})

function HistoryRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <WatchHistoryPage
      page={search.page ?? 1}
      group={search.group ?? 0}
      onPageChange={page => void navigate({ search: { ...search, page }, resetScroll: false })}
      // Switching groups returns to the first page: the new list is shorter.
      onGroupChange={group =>
        void navigate({
          search: group > 0 ? { group } : {},
          resetScroll: false
        })
      }
    />
  )
}
